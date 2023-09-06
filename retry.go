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
	"go.uber.org/zap"
)

func Retry(url string, keys utils.Keys, config model.Network, store store.Store, logger *zap.Logger) *cobra.Command {
	var (
		account uint32
		orderId uint
	)

	var cmd = &cobra.Command{
		Use:   "retry",
		Short: "Retry an order",
		Run: func(c *cobra.Command, args []string) {
			childLogger := logger.With(zap.Uint32("account", account))

			key, err := keys.GetKey(model.Ethereum, account, 0)
			if err != nil {
				childLogger.Error("failed to get the signing key:", zap.Error(err))
				return
			}
			privKey, err := key.ECDSA()
			if err != nil {
				childLogger.Error("failed to get the signing key:", zap.Error(err))
				return
			}
			signer := crypto.PubkeyToAddress(privKey.PublicKey)

			client := rest.NewClient(fmt.Sprintf("http://%s", url), hex.EncodeToString(crypto.FromECDSA(privKey)))
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

			grandChildLogger := childLogger.With(zap.Uint("order id", order.ID), zap.String("SecHash", order.SecretHash))
			execute(order, grandChildLogger, signer, keys, account, config, store.UserStore(account), utils.GetIWConfig(false))
		},
		DisableAutoGenTag: true,
	}

	cmd.Flags().Uint32Var(&account, "account", 0, "account")
	cmd.MarkFlagRequired("account")
	cmd.Flags().UintVar(&orderId, "order-id", 0, "order id")
	cmd.MarkFlagRequired("order-id")
	return cmd
}
