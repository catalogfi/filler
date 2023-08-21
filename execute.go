package cobi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/blockchain"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

func Execute(keys utils.Keys, account uint32, url string, store store.UserStore, config model.Network, logger *zap.Logger, isIw bool) {
	childLogger := logger.With(zap.Uint32("account", account))

	// get the signer key
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

	for {
		// connect to the websocket and subscribe on the signer's address
		client, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("wss://%s/ws/orders", url), nil)
		if err != nil {
			childLogger.Error("failed to dial", zap.Error(err), zap.String("executor", signer.Hex()))
			break
		}

		if err := client.WriteMessage(websocket.BinaryMessage, []byte(fmt.Sprintf("subscribe:%v", signer))); err != nil {
			childLogger.Error("failed to subscribe to the events of the user", zap.Error(err), zap.String("executor", signer.Hex()))
			break
		}

		for {
			// listen to new orders from the orderbook
			_, msg, err := client.ReadMessage()
			if err != nil {
				childLogger.Error("failed to read messege from the websocket", zap.Error(err))
				break
			}
			var orders []model.Order
			if err := json.Unmarshal(msg, &orders); err != nil {
				childLogger.Error("failed to unmarshal orders recived on the websocket", zap.String("message", string(msg)), zap.Error(err))
				break
			}

			// execute orders
			childLogger.Info("recieved orders from the order book", zap.Int("count", len(orders)))
			for _, order := range orders {
				grandChildLogger := childLogger.With(zap.Uint("order id", order.ID), zap.String("pair", order.OrderPair))
				execute(order, grandChildLogger, signer, keys, account, config, store, isIw)
			}
			childLogger.Info("executed orders recieved from the order book", zap.Int("count", len(orders)))
		}
	}
}

func execute(order model.Order, logger *zap.Logger, signer common.Address, keys utils.Keys, account uint32, config model.Network, userStore store.UserStore, isIw bool) {
	logger.Info("processing order with id", zap.Uint("status", uint(order.Status)))
	if isValid, err := userStore.CheckStatus(order.SecretHash); !isValid {
		if err != "" {
			logger.Error("failed to check status", zap.Error(errors.New(err)))
		} else {
			logger.Info("skipping order as it failed earlier")
		}
		return
	}

	status := userStore.Status(order.SecretHash)
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
		if order.Status == model.Filled {
			if status != store.InitiatorInitiated {
				handleInitiate(*order.InitiatorAtomicSwap, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator initiate")), true, isIw)
			}
		} else if order.FollowerAtomicSwap.Status == model.Detected {
			logger.Info("detected follower atomic swap", zap.String("txHash", order.FollowerAtomicSwap.InitiateTxHash))
		} else if order.FollowerAtomicSwap.Status == model.Initiated {
			if status != store.InitiatorRedeemed {
				secret, err := userStore.Secret(order.SecretHash)
				if err != nil {
					logger.Error("failed to retrieve the secret from db", zap.Error(err))
					return
				}
				handleRedeem(*order.FollowerAtomicSwap, secret, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator redeem")), true, isIw)
			}
		} else if order.InitiatorAtomicSwap.Status == model.Expired {
			if status == store.InitiatorInitiated {
				// assuming that the function would just return nil if the swap has not expired yet
				handleRefund(*order.InitiatorAtomicSwap, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator refund")), true, isIw)
			}
		}
	} else if strings.EqualFold(order.Taker, signer.Hex()) {
		if order.InitiatorAtomicSwap.Status == model.Initiated {
			if status != store.FollowerInitiated {
				handleInitiate(*order.FollowerAtomicSwap, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "follower initiate")), false, isIw)
			}
		} else if order.InitiatorAtomicSwap.Status == model.Detected {
			logger.Info("detected initiator atomic swap", zap.String("txHash", order.InitiatorAtomicSwap.InitiateTxHash))
		} else if order.FollowerAtomicSwap.Status == model.Redeemed {
			if status != store.FollowerRedeemed {
				handleRedeem(*order.InitiatorAtomicSwap, order.Secret, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "follower redeem")), false, isIw)
			}
		} else if order.FollowerAtomicSwap.Status == model.Expired {
			// assuming that the function would just return nil if the swap has not expired yet
			if status == store.FollowerInitiated {
				handleRefund(*order.FollowerAtomicSwap, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "follower refund")), false, isIw)
			}
		}
	}
}

func handleRedeem(atomicSwap model.AtomicSwap, secret, secretHash string, keyInterface interface{}, config model.Network, userStore store.UserStore, logger *zap.Logger, isInitiator bool, isIw bool) {
	logger.Info("redeeming an order")
	redeemerSwap, err := blockchain.LoadRedeemerSwap(atomicSwap, keyInterface, secretHash, config, uint64(0), isIw)
	if err != nil {
		logger.Error("failed to load redeemer swap", zap.Error(err))
		return
	}

	secretBytes, err := hex.DecodeString(secret)
	if err != nil {
		logger.Error("failed to decode secret", zap.Error(err))
		return
	}

	txHash, err := redeemerSwap.Redeem(secretBytes)
	if err != nil {
		status := store.InitiatorFailedToRedeem
		if !isInitiator {
			status = store.FollowerFailedToRedeem
		}
		if err2 := userStore.PutError(secretHash, err.Error(), status); err2 != nil {
			logger.Error("failed to store redeem error", zap.Error(err2), zap.NamedError("redeem error", err))
		} else {
			logger.Error("failed to redeem", zap.Error(err))
		}
		return
	}
	logger.Info("successfully redeemed swap", zap.String("tx hash", txHash))

	status := store.InitiatorRedeemed
	if !isInitiator {
		status = store.FollowerRedeemed
	}
	if err := userStore.PutStatus(secretHash, status); err != nil {
		logger.Error("failed to update status", zap.Error(err))
	}
}

func handleInitiate(atomicSwap model.AtomicSwap, secretHash string, keyInterface interface{}, config model.Network, userStore store.UserStore, logger *zap.Logger, isInitiator bool, isIw bool) {
	logger.Info("initiating an order")
	initiatorSwap, err := blockchain.LoadInitiatorSwap(atomicSwap, keyInterface, secretHash, config, uint64(0), isIw)
	if err != nil {
		logger.Error("failed to load initiator swap", zap.Error(err))
		return
	}

	txHash, err := initiatorSwap.Initiate()
	if err != nil {
		status := store.InitiatorFailedToInitiate
		if !isInitiator {
			status = store.FollowerFailedToInitiate
		}

		if err2 := userStore.PutError(secretHash, err.Error(), status); err2 != nil {
			logger.Error("failed to store initiate error", zap.Error(err2), zap.NamedError("initiate error", err))
		} else {
			logger.Error("failed to initiate", zap.Error(err))
		}
		return
	}
	logger.Info("successfully initiated swap", zap.String("tx hash", txHash))

	status := store.InitiatorInitiated
	if !isInitiator {
		status = store.FollowerInitiated
	}
	if err := userStore.PutStatus(secretHash, status); err != nil {
		logger.Error("failed to update status", zap.Error(err))
	}
}

func handleRefund(swap model.AtomicSwap, secretHash string, keyInterface interface{}, config model.Network, userStore store.UserStore, logger *zap.Logger, isInitiator bool, isIw bool) {
	initiatorSwap, err := blockchain.LoadInitiatorSwap(swap, keyInterface, secretHash, config, uint64(0), isIw)
	if err != nil {
		logger.Error("failed to load initiator swap", zap.Error(err))
		return
	}
	isExpired, err := initiatorSwap.Expired()
	if err != nil {
		logger.Error("failed to check if the initiator swap expired or not", zap.Error(err))
		return
	}
	if isExpired {
		logger.Info("refunding an order")
		txHash, err := initiatorSwap.Refund()
		if err != nil {
			status := store.InitiatorFailedToRefund
			if !isInitiator {
				status = store.FollowerFailedToRefund
			}

			if err2 := userStore.PutError(secretHash, err.Error(), status); err2 != nil {
				logger.Error("failed to store refund error", zap.Error(err2), zap.NamedError("refund error", err))
				return
			}
			logger.Error("failed to refund", zap.Error(err))
			return
		}
		logger.Info("successfully refunded swap", zap.String("tx hash", txHash))

		status := store.InitiatorRefunded
		if !isInitiator {
			status = store.FollowerRefunded
		}
		if err := userStore.PutStatus(secretHash, status); err != nil {
			logger.Error("failed to update status", zap.Error(err))
		}
	}
}
