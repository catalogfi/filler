package cobictl

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/catalogfi/cobi/cobid/types"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
)

func Accounts(rpcClient Client) *cobra.Command {
	var (
		user    uint32
		asset   string
		page    int
		perPage int
		useIw   bool
	)
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List account addresses and balances",
		Run: func(c *cobra.Command, args []string) {

			AccountReq := types.RequestAccount{
				IsInstantWallet: useIw,
				Asset:           asset,
				Page:            uint32(page),
				PerPage:         uint32(perPage),
				UserAccount:     user,
			}

			resp, err := rpcClient.GetAccounts(AccountReq)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}

			var accounts []types.AccountInfo
			if err := json.Unmarshal(resp, &accounts); err != nil {
				cobra.CheckErr(fmt.Errorf("failed to unmarshal response: %w", err))
			}

			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"#", "Address", "Current Balance", "Usable Balance"})
			rows := make([]table.Row, 0)
			for _, account := range accounts {
				row := table.Row{account.AccountNo, account.Address, account.Balance, account.UsableBalance}
				rows = append(rows, row)
			}
			t.AppendRows(rows)
			t.Render()
		},
		DisableAutoGenTag: true,
	}
	cmd.Flags().BoolVarP(&useIw, "instant-wallet", "i", false, "user can specify to use catalog instant wallets")
	cmd.Flags().StringVarP(&asset, "asset", "a", "", "user should provide the asset")
	cmd.MarkFlagRequired("asset")
	cmd.Flags().Uint32Var(&user, "account", 0, "user can provide the user id")
	cmd.Flags().IntVar(&perPage, "per-page", 10, "User can provide number of accounts to display per page")
	cmd.Flags().IntVar(&page, "page", 1, "User can provide which page to display")
	return cmd
}
