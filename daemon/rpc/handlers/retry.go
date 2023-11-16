package handlers

// import (
// 	"encoding/hex"
// 	"fmt"
// 	"time"

// 	storeType "github.com/catalogfi/cobi/store"
// 	"github.com/catalogfi/cobi/utils"
// 	"github.com/catalogfi/wbtc-garden/model"
// 	"github.com/catalogfi/wbtc-garden/rest"
// 	"github.com/catalogfi/cobi/pkg/swapper/bitcoin"
// 	"github.com/ethereum/go-ethereum/crypto"
// 	"github.com/spf13/cobra"
// 	"go.uber.org/zap"
// 	"gorm.io/driver/postgres"
// 	"gorm.io/gorm"
// 	glogger "gorm.io/gorm/logger"
// )

// func Retry(url string, keys utils.Keys, config model.Network, store storeType.Store, logger *zap.Logger, db string) *cobra.Command {
// 	var (
// 		account uint32
// 		orderId uint
// 		useIw   bool
// 	)

// 	var cmd = &cobra.Command{
// 		Use:   "retry",
// 		Short: "Retry an order",
// 		Run: func(c *cobra.Command, args []string) {
// 			childLogger := logger.With(zap.Uint32("account", account))

// 			key, err := keys.GetKey(model.Ethereum, account, 0)
// 			if err != nil {
// 				childLogger.Error("failed to get the signing key:", zap.Error(err))
// 				return
// 			}
// 			privKey, err := key.ECDSA()
// 			if err != nil {
// 				childLogger.Error("failed to get the signing key:", zap.Error(err))
// 				return
// 			}
// 			signer := crypto.PubkeyToAddress(privKey.PublicKey)

// 			client := rest.NewClient(fmt.Sprintf("https://%s", url), hex.EncodeToString(crypto.FromECDSA(privKey)))
// 			token, err := client.Login()
// 			if err != nil {
// 				cobra.CheckErr(fmt.Sprintf("error while getting the signing key: %v", err))
// 				return
// 			}
// 			if err := client.SetJwt(token); err != nil {
// 				cobra.CheckErr(fmt.Sprintf("error to parse signing key: %v", err))
// 				return
// 			}
// 			order, err := client.GetOrder(orderId)

// 			if err != nil {
// 				cobra.CheckErr(fmt.Sprintf("error while getting order from server: %v", err))
// 				return
// 			}

// 			accountStore := store.UserStore(account)
// 			localOrder, err := accountStore.GetOrder(orderId)
// 			if err != nil {
// 				cobra.CheckErr(fmt.Sprintf("error while loading order from local state: %v", err))
// 				return
// 			}
// 			status := localOrder.Status
// 			var updatedStatus storeType.Status
// 			switch status {
// 			case storeType.InitiatorFailedToInitiate:
// 				updatedStatus = storeType.InitiatorInitiated - 1
// 			case storeType.FollowerFailedToInitiate:
// 				updatedStatus = storeType.FollowerInitiated - 1
// 			case storeType.InitiatorFailedToRedeem:
// 				updatedStatus = storeType.InitiatorRedeemed - 1
// 			case storeType.FollowerFailedToRedeem:
// 				updatedStatus = storeType.FollowerRedeemed - 1
// 			case storeType.InitiatorFailedToRefund:
// 				if localOrder.InitiateTxHash == "" {
// 					cobra.CheckErr(fmt.Errorf("could not find initiator's initiate tx hash for the order"))
// 					return
// 				}
// 				updatedStatus = storeType.InitiatorInitiated
// 			case storeType.FollowerFailedToRefund:
// 				if localOrder.InitiateTxHash == "" {
// 					cobra.CheckErr(fmt.Errorf("could not find follower's initiate tx hash for the order"))
// 					return
// 				}
// 				updatedStatus = storeType.FollowerInitiated
// 			}
// 			err = accountStore.PutStatus(order.SecretHash, updatedStatus)
// 			if err != nil {
// 				cobra.CheckErr(fmt.Sprintf("error while parsing order pair: %v", err))
// 				return
// 			}

// 			grandChildLogger := childLogger.With(zap.Uint("order id", order.ID), zap.String("SecHash", order.SecretHash))
// 			iwStore, _ := bitcoin.NewStore(nil)
// 			if useIw {
// 				iwStore, err = bitcoin.NewStore(postgres.Open(db), &gorm.Config{
// 					NowFunc: func() time.Time { return time.Now().UTC() },
// 					Logger:  glogger.Default.LogMode(glogger.Silent),
// 				})
// 				if err != nil {
// 					cobra.CheckErr(fmt.Sprintf("Could not load iw store: %v", err))
// 					return
// 				}
// 			}
// 			execute(order, grandChildLogger, signer, keys, account, config, accountStore, iwStore)
// 		},
// 		DisableAutoGenTag: true,
// 	}

// 	cmd.Flags().Uint32Var(&account, "account", 0, "account")
// 	cmd.MarkFlagRequired("account")
// 	cmd.Flags().UintVar(&orderId, "order-id", 0, "order id")
// 	cmd.MarkFlagRequired("order-id")
// 	cmd.Flags().BoolVarP(&useIw, "instant-wallet", "i", false, "user can specify to use catalog instant wallets")
// 	return cmd
// }
