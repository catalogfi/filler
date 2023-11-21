package commands

import (
	"fmt"

	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/rpcclient"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func Transfer(rpcClient rpcclient.Client , logger *zap.Logger) *cobra.Command {
	var (
		user   uint32
		amount uint32
		asset  string
		toAddr string
		useIw  bool
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "transfer funds",
		Run: func(c *cobra.Command, args []string) {
			Transfer := types.RequestTransfer{
				UserAccount: user,
				Asset:       asset,
				Amount:      uint64(amount),
				ToAddr:      toAddr,
				UseIw:       useIw,
				Force:       force,
			}

			resp, err := rpcClient.Transfer(Transfer)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}

			logger.Info("Successfully transferred" , zap.String("txHash" ,string(resp)))
		},
		DisableAutoGenTag: true,
	}
	cmd.Flags().BoolVarP(&useIw, "instant-wallet", "i", false, "user can specify to use catalog instant wallets")
	cmd.Flags().BoolVarP(&force, "force-send", "f", false, "can force send")
	cmd.Flags().StringVarP(&asset, "asset", "a", "", "user should provide the asset")
	cmd.MarkFlagRequired("asset")
	cmd.Flags().StringVar(&toAddr, "toAddress", "", "user should provide the asset")
	cmd.MarkFlagRequired("address")
	cmd.Flags().Uint32Var(&user, "account", 0, "user can provide the user id")
	cmd.Flags().Uint32Var(&amount, "amount", 0, "amount to transfer")
	cmd.MarkFlagRequired("amount")

	return cmd
}
