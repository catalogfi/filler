package cobictl

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/catalogfi/cobi/cobid/handlers"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
)

func List(rpcClient Client) *cobra.Command {
	var (
		// url        string
		maker      string
		orderPair  string
		secretHash string
		orderBy    string
		minPrice   float64
		maxPrice   float64
		page       int
		perPage    int
	)

	var cmd = &cobra.Command{
		Use:   "list",
		Short: "List all open orders in the orderbook",
		Run: func(c *cobra.Command, args []string) {
			QueryAccount := handlers.RequestListOrders{
				Maker:      maker,
				OrderPair:  orderPair,
				SecretHash: secretHash,
				OrderBy:    orderBy,
				MinPrice:   minPrice,
				MaxPrice:   maxPrice,
				Page:       uint32(page),
				PerPage:    uint32(perPage),
			}

			jsonData, err := json.Marshal(QueryAccount)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to marshal payload: %w", err))
			}

			resp, err := rpcClient.SendPostRequest("listOrders", jsonData)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}

			var orders []model.Order
			if err := json.Unmarshal(resp, &orders); err != nil {
				cobra.CheckErr(fmt.Errorf("failed to unmarshal response: %w", err))
			}

			t := table.NewWriter()
			t.SetStyle(table.StyleRounded)
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"Order ID", "From Asset", "To Asset", "Price", "From Amount", "To Amount"})
			rows := make([]table.Row, len(orders))
			for i, order := range orders {
				assets := strings.Split(order.OrderPair, "-")
				rows[i] = table.Row{order.ID, assets[0], assets[1], order.Price, order.InitiatorAtomicSwap.Amount, order.FollowerAtomicSwap.Amount}
			}
			t.AppendRows(rows)
			t.Render()
		},
		DisableAutoGenTag: true,
	}

	cmd.Flags().StringVar(&maker, "maker", "", "maker address to filter with (default: any)")
	cmd.Flags().StringVar(&orderPair, "order-pair", "", "order pair to filter with (default: any)")
	cmd.Flags().StringVar(&secretHash, "secret-hash", "", "secret-hash to filter with (default: any)")
	cmd.Flags().StringVar(&orderBy, "order-by", "", "order by (default: creation time)")
	cmd.Flags().Float64Var(&minPrice, "min-price", 0, "minimum price to filter with (default: any)")
	cmd.Flags().Float64Var(&maxPrice, "max-price", 0, "maximum price to filter with (default: any)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (default: 0)")
	cmd.Flags().IntVar(&perPage, "per-page", 10, "per page number (default: 10)")
	return cmd
}
