package cobi

import (
	"fmt"

	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/blockchain"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/swapper/bitcoin"
	"github.com/spf13/cobra"
)

func Deposit(entropy []byte, config model.Network) *cobra.Command {
	var (
		asset   string
		account uint32
		amount  uint32
	)
	var cmd = &cobra.Command{
		Use:   "deposit",
		Short: "deposit funds from EOA to instant wallets",
		Run: func(c *cobra.Command, args []string) {

			chain, _, err := model.ParseChainAsset(asset)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while generating secret: %v", err))
				return
			}
			client, err := blockchain.LoadClient(chain, config, true)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("failed to load client: %v", err))
				return

			}
			switch client := client.(type) {
			case bitcoin.InstantClient:
				key, err := utils.LoadKey(entropy, chain, account, 0)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while getting the signing key: %v", err))
					return
				}

				privKey := key.BtcKey()
				txHash, err := client.FundInstanstWallet(privKey, int64(amount))
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("failed to deposit to instant wallet: %v", err))
					return

				}
				fmt.Println("Bitcoin deposit successful", txHash)
			}

		}}
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().Uint32Var(&amount, "amount", 0, "User should provide the amount to deposit to instant wallet")
	cmd.MarkFlagRequired("amount")
	cmd.Flags().StringVarP(&asset, "asset", "a", "", "user should provide the asset")
	cmd.MarkFlagRequired("asset")
	return cmd
}
