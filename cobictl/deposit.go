package cobictl

import (
	"fmt"

	"github.com/catalogfi/cobi/cobid/handlers"
	"github.com/spf13/cobra"
)

func Deposit(rpcClient Client) *cobra.Command {
	var (
		asset   string
		account uint32
		amount  uint64
	)
	var cmd = &cobra.Command{
		Use:   "deposit",
		Short: "deposit funds from EOA to instant wallets",
		Run: func(c *cobra.Command, args []string) {
			Deposit := handlers.RequestDeposit{
				UserAccount: account,
				Amount:      amount,
				Asset:       asset,
			}

			resp, err := rpcClient.Deposit(Deposit)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}

			fmt.Println("Funds Deposit Was SuccessFull" + string(resp))

		}}
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().Uint64Var(&amount, "amount", 0, "User should provide the amount to deposit to instant wallet")
	cmd.MarkFlagRequired("amount")
	cmd.Flags().StringVarP(&asset, "asset", "a", "", "user should provide the asset")
	cmd.MarkFlagRequired("asset")
	return cmd
}
