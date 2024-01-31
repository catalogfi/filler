package executor

import (
	"context"
	"encoding/hex"
	"errors"
	"log"
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
	ticker := time.NewTicker(90 * time.Second)
	go func() {
		defer ticker.Stop()

	outer:
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

				// Get data for previous batched orders
				bd, err := be.store.GetBatchData()
				if err != nil {
					be.logger.Error("get batch data", zap.Error(err))
					continue
				}

				isInitiated := func(swap btcswap.Swap) (bool, error) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					initiated, _, err := swap.Initiated(ctx, be.wallet.Indexer())
					return initiated, err
				}

				// Get all the new orders we need to execute.
				newActions := make([]btcswap.ActionItem, 0, len(orders))
				for _, order := range orders {
					var atomicSwap *model.AtomicSwap
					var action swap.Action

					iStatus := order.InitiatorAtomicSwap.Status
					fStatus := order.FollowerAtomicSwap.Status

					switch {
					case order.FollowerAtomicSwap.Chain == be.chain:
						// Initiate or Refund
						atomicSwap = order.FollowerAtomicSwap
						if iStatus == model.Initiated && fStatus == model.NotStarted {
							action = swap.ActionInitiate
						} else if fStatus == model.Expired {
							action = swap.ActionRefund
						} else {
							continue
						}
					case order.InitiatorAtomicSwap.Chain == be.chain:
						// Redeem
						if (fStatus == model.Redeemed || fStatus == model.RedeemDetected) && iStatus == model.Initiated {
							action = swap.ActionRedeem
							atomicSwap = order.InitiatorAtomicSwap
							order.InitiatorAtomicSwap.Secret = order.FollowerAtomicSwap.Secret
						} else {
							continue
						}
					default:
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
					actionItem := btcswap.ActionItem{
						Action:     action,
						AtomicSwap: btcSwap,
						Secret:     secret,
					}

					if bd.HasAction(actionItem) {
						continue
					}

					// Check if the swap has been initiated before to prevent double initiations.
					if action == swap.ActionInitiate {
						initiated, err := isInitiated(btcSwap)
						if err != nil {
							be.logger.Error("check swap initiation", zap.Error(err))
							continue
						}
						if initiated {
							continue
						}
					}
					newActions = append(newActions, actionItem)
				}
				be.logger.Debug("btc executor", zap.Int("new actions", len(newActions)))
				for _, actionItem := range newActions {
					be.logger.Debug("btc executor", zap.String(string(actionItem.Action), actionItem.AtomicSwap.Address.EncodeAddress()))
				}

				// Skip if we have no orders to process
				if len(newActions) == 0 {
					continue
				}

				// Submit the transaction
				log.Printf("execute rbf = %+v", bd.RbfOptions)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				txid, newRBF, err := be.wallet.ExecuteRbf(ctx, newActions, bd.RbfOptions)
				cancel()
				if err != nil {
					// Previous tx been included in the block, we wait for orderbook to update the order status and
					// check again later
					if errors.Is(err, btc.ErrTxInputsMissingOrSpent) {
						be.logger.Debug("input conflicts")
						if err := be.store.StoreBatchData(NewBatchData()); err != nil {
							be.logger.Error("store rbf info", zap.Error(err))
						}
						continue
					}
					be.logger.Error("❌ [Execution] btc ", zap.Error(err))
					continue
				}

				be.logger.Info("✅ [Execution]", zap.String("chain", "btc"), zap.String("txid", txid))

				// Update the batch data and store it for next poll
				for _, action := range newActions {
					bd.AddExecuteAction(action)
				}
				bd.RbfOptions = newRBF
				if err := be.store.StoreBatchData(bd); err != nil {
					be.logger.Error("storing batch data", zap.Error(err))
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
