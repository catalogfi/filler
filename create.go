package cobi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
)

func Create(keys utils.Keys, store store.Store, config model.Network) *cobra.Command {
	var (
		account       uint32
		url           string
		orderPair     string
		sendAmount    string
		receiveAmount string
	)

	var cmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new order",
		Run: func(c *cobra.Command, args []string) {
			secret := [32]byte{}
			if _, err := rand.Read(secret[:]); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while generating secret: %v", err))
				return
			}
			hash := sha256.Sum256(secret[:])
			secretHash := hex.EncodeToString(hash[:])
			iwConfig := utils.GetIWConfig(false)
			userStore := store.UserStore(account)
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

			fromChain, toChain, _, _, err := model.ParseOrderPair(orderPair)
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
			toKey, err := keys.GetKey(fromChain, account, 0)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting to key: %v", err))
				return
			}
			toAddress, err := toKey.Address(toChain, config, iwConfig)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
				return
			}

			id, err := client.CreateOrder(fromAddress, toAddress, orderPair, sendAmount, receiveAmount, secretHash)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while creating order: %v", err))
				return
			}

			if err = userStore.PutSecret(secretHash, hex.EncodeToString(secret[:]), uint64(id)); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while creating secret store: %v", err))
				return
			}

			fmt.Println("Order created with id: ", id)
		},
	}

	cmd.Flags().StringVar(&url, "url", "", "URL of the orderbook server")
	cmd.MarkFlagRequired("url")
	cmd.Flags().Uint32Var(&account, "account", 0, "Account to be used (default: 0)")
	cmd.Flags().StringVar(&orderPair, "order-pair", "", "User should provide the order pair")
	cmd.MarkFlagRequired("order-pair")
	cmd.Flags().StringVar(&sendAmount, "send-amount", "", "User should provide the send amount")
	cmd.MarkFlagRequired("send-amount")
	cmd.Flags().StringVar(&receiveAmount, "receive-amount", "", "User should provide the receive amount")
	cmd.MarkFlagRequired("receive-amount")
	return cmd
}
