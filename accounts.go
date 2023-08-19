package cobi

import (
	"fmt"
	"os"

	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
)

func Accounts(keys utils.Keys, config model.Config) *cobra.Command {
	var (
		url     string
		user    uint32
		asset   string
		page    int
		perPage int
	)
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List account addresses and balances",
		Run: func(c *cobra.Command, args []string) {
			ch, a, err := model.ParseChainAsset(asset)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while generating secret: %v", err))
				return
			}

			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"#", "Address", "Current Balance", "Usable Balance"})
			rows := make([]table.Row, 0)
			for i := perPage*page - perPage; i < perPage*page; i++ {
				key, err := keys.GetKey(ch, user, uint32(i))
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error parsing key: %v", err))
					return
				}
				addr, err := key.Address(ch)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error parsing address: %v", err))
					return
				}
				balance, err := utils.Balance(ch, addr, config, a)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error fetching balance: %v", err))
					return
				}

				signingKey, err := keys.GetKey(model.Ethereum, user, uint32(i))
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error getting signing key: %v", err))
					return
				}
				ecdsaKey, err := signingKey.ECDSA()
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error calculating ECDSA key: %v", err))
					return
				}

				client := rest.NewClient(url, ecdsaKey.D.Text(16))
				token, err := client.Login()
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("failed to get auth token: %v", err))
					return
				}
				if err := client.SetJwt(token); err != nil {
					cobra.CheckErr(fmt.Sprintf("failed to set auth token: %v", err))
					return
				}
				signer, err := key.EvmAddress()
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("failed to calculate evm address: %v", err))
					return
				}
				usableBalance, err := utils.VirtualBalance(ch, addr, config, a, signer.Hex(), client)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("failed to get usable balance: %v", err))
					return
				}
				row := table.Row{i, addr, balance, usableBalance}
				rows = append(rows, row)
			}
			t.AppendRows(rows)
			t.Render()
		},
		DisableAutoGenTag: true,
	}
	cmd.Flags().StringVarP(&asset, "asset", "a", "", "user should provide the asset")
	cmd.MarkFlagRequired("asset")
	cmd.Flags().StringVar(&url, "url", "", "user should provide the orderbook url")
	cmd.MarkFlagRequired("url")
	cmd.Flags().Uint32Var(&user, "account", 0, "user can provide the user id")
	cmd.Flags().IntVar(&perPage, "per-page", 10, "User can provide number of accounts to display per page")
	cmd.Flags().IntVar(&page, "page", 1, "User can provide which page to display")
	return cmd
}
