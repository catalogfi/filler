package executor

import (
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/swap"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/orderbook/model"
	"go.uber.org/zap"
)

type BitcoinExecutor struct {
	logger    *zap.Logger
	chain     model.Chain
	wallet    btcswap.Wallet
	storage   Store
	swapsChan chan ActionItem

	mu    *sync.Mutex
	swaps []btcswap.ActionItem
}

func NewBitcoinExecutor(chain model.Chain, logger *zap.Logger, wallet btcswap.Wallet, storage Store) *BitcoinExecutor {
	swapsChan := make(chan ActionItem, 16)
	exe := &BitcoinExecutor{
		logger:    logger,
		chain:     chain,
		wallet:    wallet,
		storage:   storage,
		swapsChan: swapsChan,

		mu:    new(sync.Mutex),
		swaps: make([]btcswap.ActionItem, 0),
	}

	ticker := time.NewTicker(time.Minute)

	go func() {
		for item := range swapsChan {
			// Check if we have done the same action before
			done, err := exe.storage.CheckAction(item.Action, item.Swap.ID)
			if err != nil {
				logger.Error("failed storing action", zap.Error(err))
				continue
			}
			if done {
				continue
			}

			btcSwap, err := btcswap.FromAtomicSwap(item.Swap)
			if err != nil {
				logger.Error("failed parse swap", zap.Error(err))
				continue
			}
			secret := []byte{}
			if item.Action == swap.ActionRedeem {
				secret, err = hex.DecodeString(item.Swap.Secret)
				if err != nil {
					logger.Error("failed decode secret", zap.Error(err))
					continue
				}
			}

			// Add the swap to the pending list
			exe.mu.Lock()
			actionItem := btcswap.ActionItem{
				Action:     item.Action,
				AtomicSwap: btcSwap,
				Secret:     secret,
			}
			exe.logger.Info("Adding new swap to execution", zap.Uint("swap id", item.Swap.ID))
			exe.swaps = append(exe.swaps, actionItem)
			exe.mu.Unlock()

			// Store the action we have done and make sure we're not doing it again
			if err := exe.storage.RecordAction(item.Action, item.Swap.ID); err != nil {
				logger.Error("failed storing action", zap.Error(err))
				continue
			}
		}
	}()

	go func() {
		defer ticker.Stop()
		for range ticker.C {
			exe.execute()
		}
	}()

	return exe
}

func (be *BitcoinExecutor) Chain() model.Chain {
	return be.chain
}

func (be *BitcoinExecutor) Execute(action swap.Action, atomicSwap *model.AtomicSwap) {
	be.swapsChan <- ActionItem{
		Action: action,
		Swap:   atomicSwap,
	}
}

func (be *BitcoinExecutor) execute() {
	ctx, cancel := context.WithTimeout(context.TODO(), time.Minute)
	defer cancel()

	be.mu.Lock()
	defer be.mu.Unlock()

	// Skip if there's nop swap to be executed
	if len(be.swaps) == 0 {
		return
	}

	txhash, err := be.wallet.BatchExecute(ctx, be.swaps)
	if err != nil {
		be.logger.Error("failed execute swaps", zap.Error(err))
		return
	}
	be.logger.Info("Execution swaps", zap.String("txhash", txhash))
	be.swaps = nil
}
