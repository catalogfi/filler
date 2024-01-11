package executor

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/catalogfi/cobi/pkg/swap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"go.uber.org/zap"
)

type EvmExecutor struct {
	chain     model.Chain
	wallet    ethswap.Wallet
	storage   Store
	logger    *zap.Logger
	swapsChan chan ActionItem
}

func NewEvmExecutor(chain model.Chain, logger *zap.Logger, wallet ethswap.Wallet, storage Store) *EvmExecutor {
	swapsChan := make(chan ActionItem, 16)
	exe := &EvmExecutor{
		chain:     chain,
		wallet:    wallet,
		storage:   storage,
		logger:    logger,
		swapsChan: swapsChan,
	}
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

			if err := exe.execute(item.Action, item.Swap); err != nil {
				logger.Error("execution failed", zap.String("chain", string(chain)), zap.Error(err))
			}

			// Store the action we have done and make sure we're not doing it again
			if err := exe.storage.RecordAction(item.Action, item.Swap.ID); err != nil {
				logger.Error("failed storing action", zap.Error(err))
				continue
			}
		}
	}()
	return exe
}

func (ee *EvmExecutor) Chain() model.Chain {
	return ee.chain
}

func (ee *EvmExecutor) Execute(action swap.Action, atomicSwap *model.AtomicSwap) {
	ee.swapsChan <- ActionItem{
		Action: action,
		Swap:   atomicSwap,
	}
}

func (ee *EvmExecutor) execute(action swap.Action, atomicSwap *model.AtomicSwap) error {
	ctx, cancel := context.WithTimeout(context.TODO(), time.Minute)
	defer cancel()

	ethSwap, err := ethswap.FromAtomicSwap(atomicSwap)
	if err != nil {
		return err
	}

	var txHash string
	switch action {
	case swap.ActionInitiate:
		txHash, err = ee.wallet.Initiate(ctx, ethSwap)
	case swap.ActionRedeem:
		var secret []byte
		secret, err = hex.DecodeString(atomicSwap.Secret)
		if err != nil {
			return err
		}
		txHash, err = ee.wallet.Redeem(ctx, ethSwap, secret)
	case swap.ActionRefund:
		txHash, err = ee.wallet.Refund(ctx, ethSwap)
	default:
		return fmt.Errorf("unknown action = %v", action)
	}
	ee.logger.Info("Execution Ethereum done", zap.String("hash", txHash))
	return err
}
