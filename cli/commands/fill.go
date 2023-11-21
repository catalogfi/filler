package commands

import (
	"fmt"

	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/rpcclient"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func Fill(rpcClient rpcclient.Client , logger *zap.Logger) *cobra.Command {
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

			_, err := rpcClient.FillOrder(FillOrder)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}

			logger.Info("Successfully filled order" )
		}}
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().UintVar(&orderId, "order-id", 0, "User should provide the order id")
	cmd.MarkFlagRequired("order-id")
	return cmd
}
