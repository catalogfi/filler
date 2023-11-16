package commands

import (
	"fmt"

	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/rpcclient"
	"github.com/spf13/cobra"
)

func Retry(rpcClient rpcclient.Client) *cobra.Command {
	var (
		orderId uint
		account uint32
		useIw   bool
	)
	var cmd = &cobra.Command{
		Use:   "retry",
		Short: "Retry an order",
		Run: func(c *cobra.Command, args []string) {
			RetryPayload := types.RequestRetry{
				OrderId:         uint64(orderId),
				Account:         account,
				IsInstantWallet: useIw,
			}

			resp, err := rpcClient.RetryOrder(RetryPayload)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}

			fmt.Println(string(resp))
		}}

	cmd.Flags().UintVar(&orderId, "order-id", 0, "User should provide the order id")
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().BoolVarP(&useIw, "instant-wallet", "i", false, "user can specify to use catalog instant wallets")
	cmd.MarkFlagRequired("order-id")
	return cmd
}
