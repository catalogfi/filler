package cobi

import (
	"fmt"
	"os"
	"sync"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func Start(keys utils.Keys, store store.Store, config model.Network, logger *zap.Logger) *cobra.Command {
	var (
		url      string
		strategy string
		useIw    bool
	)

	var cmd = &cobra.Command{
		Use:   "start",
		Short: "Start the atomic swap executor",
		Run: func(c *cobra.Command, args []string) {
			strategyData, err := os.ReadFile(strategy)
			if err != nil {
				cobra.CheckErr(err)
			}
			iwConfig := utils.GetIWConfig(useIw)
			start(url, keys, strategyData, config, store, logger, iwConfig)
		},
		DisableAutoGenTag: true,
	}
	cmd.Flags().BoolVarP(&useIw, "instant-wallet", "i", false, "user can specify to use catalog instant wallets")
	cmd.Flags().StringVar(&url, "url", "", "url of the orderbook")
	cmd.MarkFlagRequired("url")
	cmd.Flags().StringVar(&strategy, "strategy", "", "strategy")
	cmd.MarkFlagRequired("strategy")
	return cmd
}

func start(url string, keys utils.Keys, strategy []byte, config model.Network, store store.Store, logger *zap.Logger, iwConfig model.InstantWalletConfig) {
	wg := new(sync.WaitGroup)
	activeAccounts := map[uint32]bool{}
	strategies, err := UnmarshalStrategy(strategy)
	if err != nil {
		logger.Error("failed to unmarshal strategy", zap.Error(err))
		return
	}
	for index, strategy := range strategies {
		if !activeAccounts[strategy.Account()] {
			wg.Add(1)
			go func(account uint32, logger *zap.Logger) {
				defer wg.Done()
				Execute(keys, account, url, store.UserStore(account), config, logger, iwConfig)
			}(strategy.Account(), logger.With(zap.Uint32("executor", strategy.Account())))
			activeAccounts[strategy.Account()] = true
		}

		childLogger := logger.With(zap.String("strategy", fmt.Sprintf("%T", strategy)), zap.String("orderPair", strategy.OrderPair()), zap.Uint32("account", strategy.Account()))
		wg.Add(1)
		go func(i int, logger *zap.Logger) {
			defer wg.Done()
			switch strategy := strategies[i].(type) {
			case AutoFillStrategy:
				RunAutoFillStrategy(url, keys, config, store, logger.With(zap.String("orderPair", strategy.OrderPair()), zap.String("priceStrategy", fmt.Sprintf("%T", strategy.PriceStrategy()))), strategy, iwConfig)
			case AutoCreateStrategy:
				RunAutoCreateStrategy(url, keys, config, store, logger.With(zap.String("orderPair", strategy.OrderPair()), zap.String("priceStrategy", fmt.Sprintf("%T", strategy.PriceStrategy()))), strategy, iwConfig)
			default:
				logger.Error("unexpected strategy")
			}
		}(index, childLogger)
	}
	wg.Wait()
}
