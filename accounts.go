package cobi

import (
	"fmt"
	"os"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
)

func Accounts(entropy []byte, config model.Config) *cobra.Command {
	var (
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

			selectors := []uint32{}
			for i := perPage*page - perPage; i < perPage*page; i++ {
				selectors = append(selectors, uint32(i))
			}

			addrs, balances, err := getBalances(entropy, ch, user, selectors, config, a)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting addresses: %v", err))
				return
			}

			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"#", "Address", "Balance"})
			rows := make([]table.Row, len(balances))
			for i, balance := range balances {
				rows[i] = table.Row{selectors[i], addrs[i], balance}
			}
			t.AppendRows(rows)
			t.Render()
		},
		DisableAutoGenTag: true,
	}
	cmd.Flags().StringVarP(&asset, "asset", "a", "", "user should provide the asset")
	cmd.MarkFlagRequired("asset")
	cmd.Flags().Uint32Var(&user, "account", 0, "user can provide the user id")
	cmd.Flags().IntVar(&perPage, "per-page", 10, "User can provide number of accounts to display per page")
	cmd.Flags().IntVar(&page, "page", 1, "User can provide which page to display")
	return cmd
}
