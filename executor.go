package cobi

import (
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func Execute(entropy []byte, store Store, config model.Config, logger *zap.Logger) *cobra.Command {
	var (
		url     string
		account uint32
	)

	var cmd = &cobra.Command{
		Use:   "start",
		Short: "Start the atomic swap executor",
		Run: func(c *cobra.Command, args []string) {
			RunExecute(entropy, account, url, store, config, logger)
		},
		DisableAutoGenTag: true,
	}
	cmd.Flags().StringVar(&url, "url", "", "url of the orderbook")
	cmd.MarkFlagRequired("url")
	cmd.Flags().Uint32Var(&account, "account", 0, "account number")
	return cmd
}
