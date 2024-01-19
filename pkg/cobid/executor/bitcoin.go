package executor

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"
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

func NewBitcoinExecutor(chain model.Chain, logger *zap.Logger, wallet btcswap.Wallet, client rest.Client, store Store, signer string) *BitcoinExecutor {
	exe := &BitcoinExecutor{
		chain:  chain,
		logger: logger,
		wallet: wallet,
		client: client,
		signer: signer,
		store:  store,
		stop:   make(chan struct{}),
	}

	return exe
}

func (be *BitcoinExecutor) Start() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()

	outer:
		for {
			initiatedCache := map[string]struct{}{}

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

				rbfOptions, prevOrders, err := be.store.GetRbfInfo()
				if err != nil {
					if !errors.Is(err, ErrNotFound) {
						be.logger.Error("get previous tx info", zap.Error(err))
						continue
					}
				}

				isInitiated := func(swap btcswap.Swap) (bool, error) {
					if _, ok := initiatedCache[string(swap.SecretHash)]; ok {
						return true, nil
					}

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					initiated, _, err := swap.Initiated(ctx, be.wallet.Indexer())
					if err != nil {
						return false, err
					}
					if initiated {
						initiatedCache[string(swap.SecretHash)] = struct{}{}
					}
					return initiated, nil
				}

				// Get all the orders we need to execute.
				pendingActions := make([]btcswap.ActionItem, 0, len(orders))
				detectedActions := make([]btcswap.ActionItem, 0, len(orders))
				orderIDs := make(map[uint]struct{})
				for _, order := range orders {
					var atomicSwap *model.AtomicSwap
					var action swap.Action
					var isDetected bool

					if order.InitiatorAtomicSwap.Status == model.Initiated &&
						(order.FollowerAtomicSwap.Status == model.NotStarted ||
							order.FollowerAtomicSwap.Status == model.Detected) &&
						order.FollowerAtomicSwap.Chain == be.chain {
						action = swap.ActionInitiate
						atomicSwap = order.FollowerAtomicSwap
						if order.FollowerAtomicSwap.Status == model.Detected {
							isDetected = true
						}
					} else if (order.InitiatorAtomicSwap.Status == model.Initiated ||
						order.InitiatorAtomicSwap.Status == model.RedeemDetected) &&
						order.FollowerAtomicSwap.Status == model.Redeemed &&
						order.InitiatorAtomicSwap.Chain == be.chain {
						action = swap.ActionRedeem
						atomicSwap = order.InitiatorAtomicSwap
						order.InitiatorAtomicSwap.Secret = order.FollowerAtomicSwap.Secret
						if order.InitiatorAtomicSwap.Status == model.RedeemDetected {
							isDetected = true
						}
					} else if (order.FollowerAtomicSwap.Status == model.Expired ||
						order.FollowerAtomicSwap.Status == model.RefundDetected) &&
						order.FollowerAtomicSwap.Chain == be.chain {
						action = swap.ActionRefund
						atomicSwap = order.FollowerAtomicSwap
						if order.FollowerAtomicSwap.Status == model.RefundDetected {
							isDetected = true
						}
					} else {
						continue
					}

					// Parse the order to an action item
					atomicSwap.SecretHash = order.SecretHash
					btcSwap, err := btcswap.FromAtomicSwap(atomicSwap)
					if err != nil {
						be.logger.Error("failed parse swap", zap.Error(err))
						continue
					}
					var secret []byte
					if action == swap.ActionRedeem {
						secret, err = hex.DecodeString(atomicSwap.Secret)
						if err != nil {
							be.logger.Error("failed decode secret", zap.Error(err))
							continue outer
						}
					}

					// Check if the swap has been initiated before to prevent double initiations.
					if action == swap.ActionInitiate {
						if rbfOptions == nil {
							initiated, err := isInitiated(btcSwap)
							if err != nil {
								be.logger.Error("check swap initiation", zap.Error(err))
								continue
							}
							if initiated {
								continue
							}
						}
					}

					// Add the order to our list
					orderIDs[order.ID] = struct{}{}
					actionItem := btcswap.ActionItem{
						Action:     action,
						AtomicSwap: btcSwap,
						Secret:     secret,
					}
					if isDetected {
						detectedActions = append(detectedActions, actionItem)
					} else {
						pendingActions = append(pendingActions, actionItem)
					}
				}

				// Skip if we have no orders to process
				if len(pendingActions) == 0 {
					if len(orderIDs) == 0 {
						initiatedCache = map[string]struct{}{}
					}
					continue
				}

				// Combine all swap actions and submit a new rbf tx
				pendingActions = append(pendingActions, detectedActions...)

				// If any old order is missing from the new order set, that means one of the previous tx has been mined,
				// and we want to start a new series of rbf txs.
				for order := range prevOrders {
					if _, ok := orderIDs[order]; !ok {
						rbfOptions = nil
						prevOrders = nil
						continue
					}
				}

				// Skip if no new orders since last time
				if reflect.DeepEqual(prevOrders, orderIDs) {
					continue
				}

				// Submit the transaction
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				tx, newRbf, err := be.wallet.BatchExecute(ctx, pendingActions, rbfOptions)
				cancel()
				if err != nil {
					// Previous tx been included in the block, we wait for orderbook to update the order status and
					// check again later
					if errors.Is(err, btc.ErrTxInputsMissingOrSpent) {
						if err := be.store.StoreRbfInfo(nil, nil); err != nil {
							be.logger.Error("store rbf info", zap.Error(err))
						}
						continue
					}
					be.logger.Error("btc execution", zap.Error(err))
				}

				// Cache the tx in case we want to replace it in the future
				if err := be.store.StoreRbfInfo(newRbf, orderIDs); err != nil {
					be.logger.Error("store rbf info", zap.Error(err))
				}

				be.logger.Info("✅ [Execution]", zap.String("chain", "btc"), zap.String("txid", tx.TxHash().String()))
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
