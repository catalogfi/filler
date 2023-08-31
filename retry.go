package cobi

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func Retry(keys utils.Keys,config model.Config,store store.Store, logger *zap.Logger) *cobra.Command {
	var (
		account uint32
		orderId uint
		url     string
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
			retryOrder(order, grandChildLogger, signer, keys, account, config, store.UserStore(account))
		},
		DisableAutoGenTag: true,
	}

	cmd.Flags().Uint32Var(&account, "account", 0, "account")
	cmd.MarkFlagRequired("account")
	cmd.Flags().UintVar(&orderId, "order-id", 0, "order id")
	cmd.MarkFlagRequired("order-id")
	cmd.Flags().StringVar(&url, "url", "", "url")
	cmd.MarkFlagRequired("url")
	return cmd
}

func retryOrder(order model.Order, logger *zap.Logger, signer common.Address, keys utils.Keys, account uint32, config model.Config, userStore store.UserStore) {
	logger.Info("processing order with id", zap.Uint("status", uint(order.Status)))
	if isValid, err := userStore.CheckRetryStatus(order.SecretHash); !isValid {
		if err != "" {
			logger.Error("failed to check status", zap.Error(errors.New(err)))
		} else {
			logger.Info("skipping order as it failed earlier")
		}
		return
	}
	// so the idea behind retry in bussiness logic is to change status in store object
	// by changing status in store object watcher will try to re-execute the order
	// in order to reset appropriate status we are subtracting 7 from current status
	// statuses are in a sequence resulting in subtraction of 7 leading to its appropriate previous status
	var status store.Status
	StoreStatus := userStore.Status(order.SecretHash)
	if uint(StoreStatus) >= 13 {
		status = StoreStatus - 10
	} else {
		status = StoreStatus - 7
	}

	fromKey, err := keys.GetKey(order.InitiatorAtomicSwap.Chain, account, 0)
	if err != nil {
		logger.Error("failed to load sender key", zap.Error(err))
		return
	}
	fromKeyInterface, err := fromKey.Interface(order.InitiatorAtomicSwap.Chain)
	if err != nil {
		logger.Error("failed to load sender key", zap.Error(err))
		return
	}

	toKey, err := keys.GetKey(order.FollowerAtomicSwap.Chain, account, 0)
	if err != nil {
		logger.Error("failed to load reciever key", zap.Error(err))
		return
	}
	toKeyInterface, err := toKey.Interface(order.FollowerAtomicSwap.Chain)
	if err != nil {
		logger.Error("failed to load reciever key", zap.Error(err))
		return
	}

	if strings.EqualFold(order.Maker, signer.Hex()) {
		if order.Status == model.OrderFilled {
			if status != store.InitiatorInitiated {
				handleInitiate(*order.InitiatorAtomicSwap, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator initiate")), true)
			}
		} else if order.Status == model.FollowerAtomicSwapInitiated {
			if status != store.InitiatorRedeemed {
				secret, err := userStore.Secret(order.SecretHash)
				if err != nil {
					logger.Error("failed to retrieve the secret from db", zap.Error(err))
					return
				}
				handleRedeem(*order.FollowerAtomicSwap, secret, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator redeem")), true)
			}
		} else if (order.Status < model.OrderExecuted || order.Status == model.OrderCancelled) && order.Status != model.InitiatorAtomicSwapRedeemed {
			if status == store.InitiatorInitiated {
				// assuming that the function would just return nil if the swap has not expired yet
				handleRefund(*order.InitiatorAtomicSwap, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator refund")), true)
			}
		}
	} else if strings.EqualFold(order.Taker, signer.Hex()) {
		if order.Status == model.InitiatorAtomicSwapInitiated {
			if status != store.FollowerInitiated {
				handleInitiate(*order.FollowerAtomicSwap, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "follower initiate")), false)
			}
		} else if order.Status == model.FollowerAtomicSwapRedeemed {
			if status != store.FollowerRedeemed {
				handleRedeem(*order.InitiatorAtomicSwap, order.Secret, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "follower redeem")), false)
			}
		} else if order.Status < model.OrderExecuted && order.Status != model.FollowerAtomicSwapRedeemed {
			// assuming that the function would just return nil if the swap has not expired yet
			if status == store.FollowerInitiated {
				handleRefund(*order.FollowerAtomicSwap, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "follower refund")), false)
			}
		}
	}
}

func Update() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "update",
		Short: "Update COBI to the latest version",
		Run: func(c *cobra.Command, args []string) {
			if err := exec.Command("curl", "https://cobi-releases.s3.ap-south-1.amazonaws.com/update.sh", "-sSfL | sh").Run(); err != nil {
				cobra.CheckErr(fmt.Sprintf("failed to update cobi : %v", err))
				return
			}
		},
		DisableAutoGenTag: true,
	}
	return cmd
}
