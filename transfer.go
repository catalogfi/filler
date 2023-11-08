package cobi

import (
	"encoding/json"
	"fmt"

	"github.com/catalogfi/cobi/cobictl"
	"github.com/catalogfi/cobi/handlers"
	"github.com/spf13/cobra"
)

func Transfer(rpcClient cobictl.Client) *cobra.Command {
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
			Transfer := handlers.RequestTransfer{
				UserAccount: user,
				Asset:       asset,
				Amount:      uint64(amount),
				ToAddr:      toAddr,
				UseIw:       useIw,
				Force:       force,
			}

			jsonData, err := json.Marshal(Transfer)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to marshal payload: %w", err))
			}

			resp, err := rpcClient.SendPostRequest("transferFunds", jsonData)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}
			fmt.Println("SuccessFully transferred" + string(resp))
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
