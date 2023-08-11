package cobi

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/spf13/cobra"
)

func Fill(entropy []byte, store Store) *cobra.Command {
	var (
		url     string
		account uint32
		orderId uint
	)
	var cmd = &cobra.Command{
		Use:   "fill",
		Short: "Fill an order",
		Run: func(c *cobra.Command, args []string) {
			vals, err := getKeys(entropy, model.Ethereum, account, []uint32{0})
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting the signing key: %v", err))
				return
			}
			privKey := vals[0].(*ecdsa.PrivateKey)
			client := rest.NewClient(url, privKey.D.Text(16))
			token, err := client.Login()
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting the signing key: %v", err))
				return
			}
			if err := client.SetJwt(token); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error to parse signing key: %v", err))
				return
			}

			order, err := client.GetOrder(orderId)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while parsing order pair: %v", err))
				return
			}

			fromChain, toChain, _, _, err := model.ParseOrderPair(order.OrderPair)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while parsing order pair: %v", err))
				return
			}

			toAddress, err := getAddressString(entropy, fromChain, account, 0)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
				return
			}

			fromAddress, err := getAddressString(entropy, toChain, account, 0)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
				return
			}

			if err := client.FillOrder(orderId, fromAddress, toAddress); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
				return
			}
			if err = store.PutSecretHash(order.SecretHash, uint64(orderId)); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while storing secret hash: %v", err))
				return
			}

			fmt.Println("Order filled successfully")
		}}
	cmd.Flags().StringVar(&url, "url", "", "config file (default is ./config.json)")
	cmd.MarkFlagRequired("url")
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().UintVar(&orderId, "order-id", 0, "User should provide the order id")
	cmd.MarkFlagRequired("order-id")
	return cmd
}
