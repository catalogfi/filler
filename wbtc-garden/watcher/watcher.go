package watcher

import (
	"context"
	"encoding/hex"
	"fmt"
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

	UpdateSwap(swap *model.AtomicSwap) error
	GetActiveSwaps(chain model.Chain) ([]model.AtomicSwap, error)
	SwapByOCID(ocID string) (model.AtomicSwap, error)
}

// Watcher watches the blockchain and update order status accordingly.
type Watcher interface {
	Run(ctx context.Context)
	RunWorker(ctx context.Context)
}

type watcher struct {
	logger  *zap.Logger
	store   Store
	config  model.Network
	workers int
	orders  chan model.Order
}

// NewWatcher returns a new Watcher
func NewWatcher(logger *zap.Logger, store Store, config model.Network, workers int) Watcher {
	return &watcher{
		logger:  logger.With(zap.String("service", "watcher")),
		store:   store,
		config:  config,
		workers: workers,
		orders:  make(chan model.Order, 32),
	}
}

func (w *watcher) Run(ctx context.Context) {
	for i := 0; i < w.workers; i++ {
		childCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go w.RunWorker(childCtx)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer close(w.orders)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if len(w.orders) == 0 {
				orders, err := w.store.GetActiveOrders()
				if err != nil {
					w.logger.Error("get active order", zap.Error(err))
					continue
				}
				for _, order := range orders {
					w.orders <- order
				}
			}
		}
	}
}

// allows for concurrent processing of orders
func (w *watcher) RunWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case order := <-w.orders:
			logger := w.logger.With(zap.Uint("order id", order.ID))
			order, hasUpdated, err := ProcessOrder(order, w.store, w.config, logger)
			if err != nil {
				logger.Error("process order failed with", zap.Error(err))
				continue
			}
			if hasUpdated {
				if err := w.store.UpdateOrder(&order); err != nil {
					logger.Error("update order failed with", zap.Error(err))
				}
			}
		}
	}
}

func ProcessOrder(order model.Order, store Store, config model.Network, logger *zap.Logger) (model.Order, bool, error) {
	// Special cases regardless of order status
	switch {
	case order.Status == model.Created && time.Since(order.CreatedAt) > OrderTimeout:
		order.Status = model.Cancelled

	// Redeem and refund happens at the same time which should not happen. Defensive check.
	case (order.InitiatorAtomicSwap.Status == model.Redeemed && order.FollowerAtomicSwap.Status == model.Refunded) || (order.FollowerAtomicSwap.Status == model.Redeemed && order.InitiatorAtomicSwap.Status == model.Refunded):
		logger.Error("atomic swap hard failed as someone both redeemed and refunded")
		order.Status = model.FailedHard
	// Follower swap never gets initiated and initiator swap is initiated and refunded or both swaps are refunded
	case (order.InitiatorAtomicSwap.Status == model.Refunded && order.FollowerAtomicSwap.Status == model.NotStarted) || (order.InitiatorAtomicSwap.Status == model.Refunded && order.FollowerAtomicSwap.Status == model.Refunded):
		logger.Error("atomic swap soft failed due to initiator refunding")
		order.Status = model.FailedSoft
	// Follower has not filled the swap before the order timeout
	case (order.Status == model.Created && time.Since(order.CreatedAt) > OrderTimeout) || order.Status == model.Filled && time.Since(order.CreatedAt) > SwapInitiationTimeout && order.InitiatorAtomicSwap.Status == model.NotStarted:
		logger.Info("atomic swap cancelled due to fill timeout")
		order.Status = model.Cancelled
	}
	if order.Status != model.Filled {
		return order, order.Status != model.Created, nil
	}

	// Fetch swapper watchers for both parties
	initiatorWatcher, err := blockchain.LoadWatcher(*order.InitiatorAtomicSwap, order.SecretHash, config, order.InitiatorAtomicSwap.MinimumConfirmations)
	if err != nil {
		return order, false, fmt.Errorf("failed to load initiator watcher: %w", err)
	}
	followerWatcher, err := blockchain.LoadWatcher(*order.FollowerAtomicSwap, order.SecretHash, config, order.FollowerAtomicSwap.MinimumConfirmations)
	if err != nil {
		return order, false, fmt.Errorf("failed to load follower watcher: %w", err)
	}

	return UpdateStatus(logger, order, initiatorWatcher, followerWatcher)
}

func UpdateSwapStatus(log *zap.Logger, swap model.AtomicSwap, watcher swapper.Watcher) (model.AtomicSwap, bool, error) {
	switch swap.Status {
	case model.NotStarted:
		fullyFilled, txHash, amount, err := watcher.IsDetected()
		if err != nil {
			return swap, false, fmt.Errorf("failed to detect swap, %w", err)
		}
		if fullyFilled {
			// AtomicSwap initiation detected
			swap.Status = model.Detected
			swap.FilledAmount = amount
			swap.InitiateTxHash = txHash
			log.Info("atomic swap detected", zap.Uint("swap id", swap.ID), zap.String("txhash", txHash), zap.String("amount", amount))
			return swap, true, nil
		}
		if swap.FilledAmount != amount || swap.InitiateTxHash != txHash {
			// AtomicSwap partial initiation detected
			swap.FilledAmount = amount
			swap.InitiateTxHash = txHash
			return swap, true, nil
		}
	case model.Detected:
		height, conf, isIw, err := watcher.Status(swap.InitiateTxHash)
		if err != nil {
			return swap, false, err
		}
		if isIw {
			swap.MinimumConfirmations = 0
			swap.IsInstantWallet = true
			swap.Status = model.Initiated
			return swap, true, nil
		}
		if swap.CurrentConfirmations != conf {
			if conf >= swap.MinimumConfirmations {
				conf = swap.MinimumConfirmations
			}
			swap.CurrentConfirmations = conf
			swap.InitiateBlockNumber = height
			if swap.CurrentConfirmations == swap.MinimumConfirmations {
				log.Info("atomic swap initiated", zap.Uint("swap id", swap.ID), zap.Uint64("confirmations", conf), zap.Uint64("height", height))
				swap.Status = model.Initiated
			}
			return swap, true, nil
		}
	case model.Initiated:
		// Swap redeemed
		redeemed, secret, txHash, err := watcher.IsRedeemed()
		if err != nil {
			return swap, false, fmt.Errorf("follower swap redeeming, %w", err)
		}
		if redeemed {
			log.Info("atomic swap redeemed", zap.Uint("swap id", swap.ID), zap.String("tx hash", txHash))
			swap.Secret = hex.EncodeToString(secret)
			swap.Status = model.Redeemed
			swap.RedeemTxHash = txHash
			return swap, true, nil
		}

		// Swap Expired
		watcher.Expired()
		expired, err := watcher.Expired()
		if err != nil {
			return swap, false, err
		}
		if expired {
			log.Info("atomic swap expired", zap.Uint("swap id", swap.ID))
			swap.Status = model.Expired
			return swap, true, nil
		}

	case model.Expired:
		// Swap expired
		refunded, txHash, err := watcher.IsRefunded()
		if err != nil {
			return swap, false, fmt.Errorf("failed to check refund status %v", err)
		}
		if refunded {
			log.Info("atomic swap expired", zap.Uint("swap id", swap.ID), zap.String("tx hash", txHash))
			swap.Status = model.Refunded
			swap.RefundTxHash = txHash
			return swap, true, nil
		}

		// Swap redeemed
		redeemed, secret, txHash, err := watcher.IsRedeemed()
		if err != nil {
			return swap, false, fmt.Errorf("follower swap redeeming, %w", err)
		}
		if redeemed {
			log.Info("atomic swap redeemed", zap.Uint("swap id", swap.ID), zap.String("tx hash", txHash))
			swap.Secret = hex.EncodeToString(secret)
			swap.Status = model.Redeemed
			swap.RedeemTxHash = txHash
			return swap, true, nil
		}
	}
	return swap, false, nil
}

func UpdateStatus(log *zap.Logger, order model.Order, initiatorWatcher, followerWatcher swapper.Watcher) (model.Order, bool, error) {
	var hasUpdated bool
	for {
		initiatorSwap, ictn, ierr := UpdateSwapStatus(log, *order.InitiatorAtomicSwap, initiatorWatcher)
		followerSwap, fctn, ferr := UpdateSwapStatus(log, *order.FollowerAtomicSwap, followerWatcher)
		if ierr != nil || ferr != nil {
			return order, false, fmt.Errorf("initiatorSwap error : %v ,followerSwap error : %v ", ierr, ferr)
		}
		if ictn || fctn {
			if order.Secret != followerSwap.Secret {
				order.Secret = followerSwap.Secret
			}
			if initiatorSwap.Status == model.Redeemed && followerSwap.Status == model.Redeemed {
				log.Info("atomic swap executed", zap.Uint("order id", order.ID))
				order.Status = model.Executed
			}
			order.InitiatorAtomicSwap = &initiatorSwap
			order.FollowerAtomicSwap = &followerSwap

			hasUpdated = true
			continue
		}
		return order, hasUpdated, nil
	}
}
