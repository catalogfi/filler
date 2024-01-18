package executor

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/pkg/swap"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"go.uber.org/zap"
)

type BitcoinExecutor struct {
	chain  model.Chain
	logger *zap.Logger
	wallet btcswap.Wallet
	client rest.Client
	signer string
	store  Store
	stop   chan struct{}
}

func NewBitcoinExecutor(chain model.Chain, logger *zap.Logger, wallet btcswap.Wallet, client rest.Client, signer string) *BitcoinExecutor {
	exe := &BitcoinExecutor{
		chain:  chain,
		logger: logger,
		wallet: wallet,
		client: client,
		signer: signer,
		stop:   make(chan struct{}),
	}

	return exe
}

func (be *BitcoinExecutor) Start() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				filter := rest.GetOrdersFilter{
					Taker:   be.signer,
					Verbose: true,
					Status:  int(model.Filled),
				}
				orders, err := be.client.GetOrders(filter)
				if err != nil {
					be.logger.Error("get filled orders", zap.Error(err))
					continue
				}

				// Get all the orders we need to execute.
				pendingActions := make([]btcswap.ActionItem, 0, len(orders))
				orderIDs := make(map[uint]struct{})
				for _, order := range orders {
					var atomicSwap *model.AtomicSwap
					var action swap.Action

					if order.InitiatorAtomicSwap.Status == model.Initiated &&
						order.FollowerAtomicSwap.Status == model.NotStarted &&
						order.FollowerAtomicSwap.Chain == be.chain {
						action = swap.ActionInitiate
						atomicSwap = order.FollowerAtomicSwap
					} else if order.InitiatorAtomicSwap.Status == model.Initiated &&
						order.FollowerAtomicSwap.Status == model.Redeemed &&
						order.InitiatorAtomicSwap.Chain == be.chain {
						action = swap.ActionRedeem
						atomicSwap = order.InitiatorAtomicSwap
						order.InitiatorAtomicSwap.Secret = order.FollowerAtomicSwap.Secret
					} else if order.FollowerAtomicSwap.Status == model.Expired &&
						order.FollowerAtomicSwap.Chain == be.chain {
						action = swap.ActionRefund
						atomicSwap = order.FollowerAtomicSwap
					} else {
						continue
					}

					// Add the action item to the list
					orderIDs[order.ID] = struct{}{}
					atomicSwap.SecretHash = order.SecretHash
					btcSwap, err := btcswap.FromAtomicSwap(order.FollowerAtomicSwap)
					if err != nil {
						be.logger.Error("failed parse swap", zap.Error(err))
						continue
					}
					var secret []byte
					if action == swap.ActionRedeem {
						secret, err = hex.DecodeString(atomicSwap.Secret)
						if err != nil {
							be.logger.Error("failed decode secret", zap.Error(err))
							continue
						}
					}

					pendingActions = append(pendingActions, btcswap.ActionItem{
						Action:     action,
						AtomicSwap: btcSwap,
						Secret:     secret,
					})
				}

				// Skip if we have no orders to process
				if len(orderIDs) == 0 {
					continue
				}

				// If any old order is missing from the new order set, that means one of the previous tx has been mined,
				// and we want to start a new series of rbf txs.
				prevFees, prevTx, prevOrders, err := be.store.GetPreviousTx()
				if err != nil {
					if !errors.Is(err, ErrNotFound) {
						be.logger.Error("get previous tx info", zap.Error(err))
						continue
					}
				}
				for order := range prevOrders {
					if _, ok := orderIDs[order]; !ok {
						if err := be.store.StorePreviousTx(0, nil, nil); err != nil {
							be.logger.Error("btc execution", zap.Error(err))
						}
						break
					}
				}

				// Skip if no new orders since last time
				if len(prevOrders) == len(orderIDs) {
					continue
				}

				// Check the previous tx and replace it
				var rbf *btcswap.OptionRBF
				if prevTx != nil {
					rbf = &btcswap.OptionRBF{
						PreviousFee:  prevFees,
						PreviousSize: btc.TxVirtualSize(prevTx),
						PreviousTx:   prevTx,
					}
				}

				// Submit the transaction
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				tx, fees, err := be.wallet.BatchExecute(ctx, pendingActions, rbf)
				cancel()
				if err != nil {
					// Previous tx been included in the block, we wait for orderbook to update the order status and
					// check again later
					if errors.Is(err, btc.ErrTxInputsMissingOrSpent) {
						continue
					}
					be.logger.Error("btc execution", zap.Error(err))
				}

				// Cache the tx in case we want to replace it in the future
				if err := be.store.StorePreviousTx(fees, tx, orderIDs); err != nil {
					be.logger.Error("btc execution", zap.Error(err))
				}
			case <-be.stop:
				return
			}
		}
	}()
}

func (be *BitcoinExecutor) Stop() {
	if be.stop != nil {
		close(be.stop)
		be.stop = nil
	}
}
