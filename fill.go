package cobi

import (
	"encoding/hex"
	"fmt"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
)

func Fill(url string, keys utils.Keys, store store.Store,config model.Network) *cobra.Command {
	var (
		account uint32
		orderId uint
	)
	var cmd = &cobra.Command{
		Use:   "fill",
		Short: "Fill an order",
		Run: func(c *cobra.Command, args []string) {
			iwConfig := utils.GetIWConfig(false)
			key, err := keys.GetKey(model.Ethereum, account, 0)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting the signing key: %v", err))
			}
			privKey, err := key.ECDSA()
			if err != nil {
				cobra.CheckErr(err)
			}
			client := rest.NewClient(fmt.Sprintf("https://%s", url), hex.EncodeToString(crypto.FromECDSA(privKey)))
			token, err := client.Login()
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting the signing key: %v", err))
				return
			}
			if err := client.SetJwt(token); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error to parse signing key: %v", err))
				return
			}
			userStore := store.UserStore(account)

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

			// Get the addresses on different chains.
			fromKey, err := keys.GetKey(fromChain, account, 0)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting from key: %v", err))
				return
			}
			fromAddress, err := fromKey.Address(fromChain, config, iwConfig)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
				return
			}
			toKey, err := keys.GetKey(toChain, account, 0)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting to key: %v", err))
				return
			}
			toAddress, err := toKey.Address(toChain, config, iwConfig)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
				return
			}

			if err := client.FillOrder(orderId, toAddress, fromAddress); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
				return
			}
			if err = userStore.PutSecretHash(order.SecretHash, uint64(orderId)); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while storing secret hash: %v", err))
				return
			}

			fmt.Println("Order filled successfully")
		}}
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().UintVar(&orderId, "order-id", 0, "User should provide the order id")
	cmd.MarkFlagRequired("order-id")
	return cmd
}
