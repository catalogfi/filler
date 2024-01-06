package executor

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/orderbook/model"
	"go.uber.org/zap"
)

type BitcoinExecutor struct {
	chain     model.Chain
	wallet    btcswap.Wallet
	storage   Store
	swapsChan chan ActionItem
}

func NewBitcoinExecutor(chain model.Chain, logger *zap.Logger, wallet btcswap.Wallet, storage Store) BitcoinExecutor {
	swapsChan := make(chan ActionItem, 16)
	exe := BitcoinExecutor{
		chain:     chain,
		wallet:    wallet,
		storage:   storage,
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

			// Execute the swap
			if err := exe.execute(item.Action, item.Swap); err != nil {
				logger.Error("execution failed", zap.String("chain", string(chain)), zap.Error(err), zap.Uint("swap id", item.Swap.ID))
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

func (be BitcoinExecutor) Chain() model.Chain {
	return be.chain
}

func (be BitcoinExecutor) Execute(action Action, atomicSwap *model.AtomicSwap) {
	be.swapsChan <- ActionItem{
		Action: action,
		Swap:   atomicSwap,
	}
}

func (be BitcoinExecutor) execute(action Action, atomicSwap *model.AtomicSwap) error {
	ctx, cancel := context.WithTimeout(context.TODO(), time.Minute)
	defer cancel()

	walletAddr := be.wallet.Address().EncodeAddress()
	swap, err := btcswap.FromAtomicSwap(atomicSwap)
	if err != nil {
		return err
	}

	switch action {
	case ActionInitiate:
		_, err = be.wallet.Initiate(ctx, swap)
	case ActionRedeem:
		secret, err := hex.DecodeString(atomicSwap.Secret)
		if err != nil {
			return err
		}
		_, err = be.wallet.Redeem(ctx, swap, secret, be.wallet.Address().String())
	case ActionRefund:
		_, err = be.wallet.Refund(ctx, swap, walletAddr)
	default:
		return fmt.Errorf("unknown action = %v", action)
	}
	return err
}
