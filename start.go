package cobi

import (
	"os"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func Execute(entropy []byte, store Store, config model.Config, logger *zap.Logger) *cobra.Command {
	var (
		url      string
		strategy string
	)

	var cmd = &cobra.Command{
		Use:   "start",
		Short: "Start the atomic swap executor",
		Run: func(c *cobra.Command, args []string) {
			strategyData, err := os.ReadFile(strategy)
			if err != nil {
				cobra.CheckErr(err)
			}
			if err := Start(url, entropy, strategyData, config, store, logger); err != nil {
				cobra.CheckErr(err)
			}
		},
		DisableAutoGenTag: true,
	}
	cmd.Flags().StringVar(&url, "url", "", "url of the orderbook")
	cmd.MarkFlagRequired("url")
	cmd.Flags().StringVar(&strategy, "strategy", "", "strategy")
	cmd.MarkFlagRequired("strategy")
	return cmd
}
