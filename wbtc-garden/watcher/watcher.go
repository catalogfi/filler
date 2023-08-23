package watcher

import (
	"encoding/hex"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/swapper"
	"go.uber.org/zap"
)

const SwapInitiationTimeout = 12 * time.Hour
const OrderTimeout = 3 * time.Minute

type Store interface {
	// UpdateOrder updates and order status in the db
	UpdateOrder(order *model.Order) error
	// GetActiveOrders fetches all orders which are active.
	GetActiveOrders() ([]model.Order, error)
}

// Watcher watches the blockchain and update order status accordingly.
type Watcher interface {
	Run()
}

type watcher struct {
	logger  *zap.Logger
	store   Store
	config  model.Config
	workers int
	orders  chan model.Order
}

// NewWatcher returns a new Watcher
func NewWatcher(logger *zap.Logger, store Store, config model.Config) Watcher {
	workers := runtime.NumCPU() * 4
	if workers == 0 {
		workers = 4
	}
	childLogger := logger.With(zap.String("service", "watcher"))
	return &watcher{
		logger:  childLogger,
		store:   store,
		config:  config,
		workers: workers,
		orders:  make(chan model.Order, 32),
	}
}

func (w *watcher) Run() {
	for i := 0; i < w.workers; i++ {
		go func() {
			for order := range w.orders {
				if err := w.watch(order); err != nil {
					w.logger.Error("update order", zap.Error(err), zap.Uint("orderID", order.ID))
				}
			}
		}()
	}

	defer close(w.orders)
	for {
		orders, err := w.store.GetActiveOrders()
		if err != nil {
			w.logger.Error("get active order", zap.Error(err))
			continue
		}
		for _, order := range orders {
			w.orders <- order
		}

		// Wait at least 5 seconds and until the channel is empty
		// reduced to test increse in performance
		time.Sleep(5 * time.Second)
		for {
			if len(w.orders) == 0 {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func (w *watcher) watch(order model.Order) error {

	// Special cases regardless of order status
	switch {
	case order.Status == model.OrderCreated && time.Since(order.CreatedAt) > OrderTimeout:
		order.Status = model.OrderCancelled

	// Redeem and refund happens at the same time which should not happen. Defensive check.
	case (order.InitiatorAtomicSwap.RedeemTxHash != "" || order.FollowerAtomicSwap.RedeemTxHash != "") && (order.FollowerAtomicSwap.RefundTxHash != "" || order.InitiatorAtomicSwap.RefundTxHash != ""):
		order.Status = model.OrderFailedHard
	// Follower swap never gets initiated and Initiator swap is initiated and refunded
	case order.InitiatorAtomicSwap.RefundTxHash != "" && order.FollowerAtomicSwap.InitiateTxHash == "" && order.InitiatorAtomicSwap.RedeemTxHash == "":
		order.Status = model.OrderFailedSoft
	// Follower swap not been redeemed and both swap been refunded
	case order.InitiatorAtomicSwap.RedeemTxHash == "" && order.FollowerAtomicSwap.RedeemTxHash == "" && order.FollowerAtomicSwap.RefundTxHash != "" && order.InitiatorAtomicSwap.RefundTxHash != "":
		order.Status = model.OrderFailedSoft
	}
	if order.Status == model.OrderFailedHard || order.Status == model.OrderFailedSoft || order.Status == model.OrderCancelled {
		return w.store.UpdateOrder(&order)
	}

	if order.Status == model.OrderCreated {
		return nil
	}

	// Fetch swapper watchers for both parties
	initiatorWatcher, err := blockchain.LoadWatcher(*order.InitiatorAtomicSwap, order.SecretHash, w.config, order.InitiatorAtomicSwap.MinimumConfirmations)
	if err != nil {
		return fmt.Errorf("load initiator watcher, %w", err)
	}
	followerWatcher, err := blockchain.LoadWatcher(*order.FollowerAtomicSwap, order.SecretHash, w.config, order.FollowerAtomicSwap.MinimumConfirmations)
	if err != nil {
		return fmt.Errorf("load follower watcher, %w", err)
	}

	// Status update
	var orderUpdate bool
	for {
		var ctn bool
		order, ctn, err = w.statusCheck(order, initiatorWatcher, followerWatcher)
		if err != nil {
			return fmt.Errorf("check status, %w", err)
		}
		if !ctn {
			break
		}
		orderUpdate = true
	}
	if orderUpdate {
		return w.store.UpdateOrder(&order)
	}
	return nil
}

func (w *watcher) statusCheck(order model.Order, initiatorWatcher, followerWatcher swapper.Watcher) (model.Order, bool, error) {
	initiatorMinRefundTime := order.CreatedAt.Add(48 * time.Hour)
	followerMinRefundTime := order.CreatedAt.Add(24 * time.Hour)

	switch order.Status {
	case model.OrderCreated:
		if time.Since(order.CreatedAt) > OrderTimeout {
			order.Status = model.OrderCancelled
			return order, true, nil
		}
	case model.OrderFilled:
		// Initiator swap initiated
		initiated, txHashes, progress, err := initiatorWatcher.IsInitiated()
		if err != nil {
			return order, false, fmt.Errorf("initiator swap initiation, %w", err)
		}
		if initiated {
			order.Status = model.InitiatorAtomicSwapInitiated
			order.InitiatorAtomicSwap.InitiateTxHash = strings.Join(txHashes, ",")
			order.InitiatorAtomicSwap.CurrentConfirmationStatus = order.InitiatorAtomicSwap.MinimumConfirmations
			return order, true, nil
		} else if order.InitiatorAtomicSwap.CurrentConfirmationStatus < progress {
			order.InitiatorAtomicSwap.CurrentConfirmationStatus = progress
			return order, true, nil
		}

		// Order expired
		if time.Since(order.CreatedAt) > SwapInitiationTimeout {
			order.Status = model.OrderCancelled
			return order, true, nil
		}
	case model.InitiatorAtomicSwapInitiated:
		// Initiator swap refunded
		if time.Now().After(initiatorMinRefundTime) {
			refunded, txHash, err := initiatorWatcher.IsRefunded()
			if err != nil {
				return order, false, fmt.Errorf("initiator swap refund, %w", err)
			}
			if refunded {
				order.Status = model.InitiatorAtomicSwapRefunded
				order.InitiatorAtomicSwap.RefundTxHash = txHash
				return order, true, nil
			}
		}

		// Follower swap initiated
		initiated, txHashes, progress, err := followerWatcher.IsInitiated()
		if err != nil {
			return order, false, fmt.Errorf("follower swap initiation, %w", err)
		}
		if initiated {
			order.Status = model.FollowerAtomicSwapInitiated
			order.FollowerAtomicSwap.InitiateTxHash = strings.Join(txHashes, ",")
			order.FollowerAtomicSwap.CurrentConfirmationStatus = order.FollowerAtomicSwap.MinimumConfirmations
			return order, true, nil
		} else if order.FollowerAtomicSwap.CurrentConfirmationStatus < progress {
			order.FollowerAtomicSwap.CurrentConfirmationStatus = progress
			return order, true, nil
		}
	case model.FollowerAtomicSwapInitiated:
		//  Check if follower swap is refunded
		if time.Now().After(followerMinRefundTime) {
			refunded, txHash, err := followerWatcher.IsRefunded()
			if err != nil {
				return order, false, fmt.Errorf("follower swap refund, %w", err)
			}
			if refunded {
				order.Status = model.FollowerAtomicSwapRefunded
				order.FollowerAtomicSwap.RefundTxHash = txHash
				return order, true, nil
			}
		}

		//  Check if initiator swap is refunded
		if time.Now().After(initiatorMinRefundTime) {
			refunded, txHash, err := initiatorWatcher.IsRefunded()
			if err != nil {
				return order, false, fmt.Errorf("initiator swap refund, %w", err)
			}
			if refunded {
				order.Status = model.InitiatorAtomicSwapRefunded
				order.InitiatorAtomicSwap.RefundTxHash = txHash
				return order, true, nil
			}
		}

		// Follower swap redeemed
		redeemed, secret, txHash, err := followerWatcher.IsRedeemed()
		if err != nil {
			return order, false, fmt.Errorf("follower swap redeeming, %w", err)
		}
		if redeemed {
			order.Secret = hex.EncodeToString(secret)
			order.Status = model.FollowerAtomicSwapRedeemed
			order.FollowerAtomicSwap.RedeemTxHash = txHash
			return order, true, nil
		}
	case model.FollowerAtomicSwapRedeemed:
		// Initiator swap refunded
		if time.Now().After(initiatorMinRefundTime) {
			refunded, txHash, err := initiatorWatcher.IsRefunded()
			if err != nil {
				return order, false, fmt.Errorf("initiator swap refund, %w", err)
			}
			if refunded {
				order.Status = model.InitiatorAtomicSwapRefunded
				order.InitiatorAtomicSwap.RefundTxHash = txHash
				return order, true, nil
			}
		}

		// Initiator swap redeemed
		redeemed, _, txHash, err := initiatorWatcher.IsRedeemed()
		if err != nil {
			return order, false, fmt.Errorf("initiator swap redeeming, %w", err)
		}
		if redeemed {
			order.Status = model.InitiatorAtomicSwapRedeemed
			order.InitiatorAtomicSwap.RedeemTxHash = txHash
			return order, true, nil
		}
	case model.InitiatorAtomicSwapRedeemed:
		order.Status = model.OrderExecuted
		return order, true, nil
	case model.FollowerAtomicSwapRefunded:
		if time.Now().After(initiatorMinRefundTime) {
			refunded, txHash, err := initiatorWatcher.IsRefunded()
			if err != nil {
				return order, false, fmt.Errorf("initiator swap refund, %w", err)
			}
			if refunded {
				order.InitiatorAtomicSwap.RefundTxHash = txHash
				return order, false, nil
			}
		}
	}

	return order, false, nil
}
