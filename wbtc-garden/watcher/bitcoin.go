package watcher

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/screener"
	"go.uber.org/zap"
)

type BTCWatcher struct {
	store    Store
	config   model.Config
	screener screener.Screener
	interval time.Duration
	logger   *zap.Logger
	chain    model.Chain
}

func NewBTCWatcher(store Store, config model.Config, screener screener.Screener, logger *zap.Logger) *BTCWatcher {
	return &BTCWatcher{
		store:    store,
		config:   config,
		logger:   logger,
		screener: screener,
		interval: 10 * time.Second,
	}
}

func (w *BTCWatcher) Watch() {
	for {
		swaps, err := w.store.GetActiveSwaps(w.chain)
		if err != nil {
			w.logger.Error("failed to fetch active orders", zap.Error(err))
			continue
		}
		for _, swap := range swaps {
			if err := w.CheckSwap(&swap); err != nil {
				w.logger.Error("failed to check swap", zap.Error(err))
				continue
			}
		}
		time.Sleep(w.interval)
	}
}

func (w *BTCWatcher) CheckSwap(swap *model.AtomicSwap) error {
	watcher, err := blockchain.LoadWatcher(*swap, swap.SecretHash, w.config.Network, swap.MinimumConfirmations)
	if err != nil {
		return err
	}

	if swap.InitiateTxHash == "" {
		initiated, txHash, initiators, confirmations, err := watcher.IsInitiated()
		if err != nil {
			return err
		}
		if !initiated {
			return nil
		}
		isBlacklisted, err := w.screener.IsBlacklisted(initiators)
		if err != nil {
			return err
		}
		if isBlacklisted {
			return fmt.Errorf("blacklisted deposits detected")
		}

		swap.InitiateTxHash = txHash
		if swap.CurrentConfirmations != confirmations {
			swap.CurrentConfirmations = confirmations
		}
		if swap.CurrentConfirmations > swap.MinimumConfirmations {
			swap.CurrentConfirmations = swap.MinimumConfirmations
		}
	}
	if swap.RedeemTxHash == "" {
		redeemed, secret, txHash, err := watcher.IsRedeemed()
		if err != nil {
			return err
		}
		if !redeemed {
			return nil
		}
		swap.RedeemTxHash = txHash
		swap.Secret = hex.EncodeToString(secret)
	}
	if swap.RefundTxHash == "" {
		refunded, txHash, err := watcher.IsRefunded()
		if err != nil {
			return err
		}
		if !refunded {
			return nil
		}
		swap.RefundTxHash = txHash
	}
	return w.store.UpdateSwap(swap)
}
