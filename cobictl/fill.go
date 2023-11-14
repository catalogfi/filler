package cobictl

import (
	"fmt"

	"github.com/catalogfi/cobi/cobid/types"
	"github.com/spf13/cobra"
)

func Fill(rpcClient Client) *cobra.Command {
	var (
		account uint32
		orderId uint
	)
	var cmd = &cobra.Command{
		Use:   "fill",
		Short: "Fill an order",
		Run: func(c *cobra.Command, args []string) {
			FillOrder := types.RequestFill{
				UserAccount: account,
				OrderId:     uint64(orderId),
			}

			resp, err := rpcClient.FillOrder(FillOrder)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}

			fmt.Println(string(resp))
		}}
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().UintVar(&orderId, "order-id", 0, "User should provide the order id")
	cmd.MarkFlagRequired("order-id")
	return cmd
}
