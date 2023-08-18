package cobi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/catalogfi/wbtc-garden/blockchain"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

func RunExecute(entropy []byte, account uint32, url string, store Store, config model.Config, logger *zap.Logger) {
	// Load keys
	keys := NewKeys()
	key, err := keys.GetKey(entropy, model.Ethereum, account, 0)
	if err != nil {
		logger.Fatal("failed to get the signing key:", zap.Error(err))
	}
	privKey, err := key.ECDSA()
	if err != nil {
		logger.Fatal("failed to get the signing key:", zap.Error(err))
	}
	makerOrTaker := crypto.PubkeyToAddress(privKey.PublicKey)

	for {
		client, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://%s/ws/orders", url), nil)
		if err != nil {
			logger.Fatal("failed to dial: ", zap.Error(err), zap.String("executor", makerOrTaker.Hex()))
		}

		if err := client.WriteMessage(websocket.BinaryMessage, []byte(fmt.Sprintf("subscribe:%v", makerOrTaker))); err != nil {
			logger.Fatal("failed to subscribe to the events of the user: ", zap.Error(err), zap.String("executor", makerOrTaker.Hex()))
		}

		for {
			logger.Info("getting orders from the orderbook ...", zap.String("executor", makerOrTaker.Hex()))
			_, msg, err := client.ReadMessage()
			if err != nil {
				logger.Info("failed to read messege from the websocket: ", zap.Error(err))
				break
			}

			var orders []model.Order
			if err := json.Unmarshal(msg, &orders); err != nil {
				logger.Info("failed to unmarshal orders recived on the websocket: ", zap.String("message", string(msg)), zap.Error(err))
				break
			}
			logger.Info("processing orders ...", zap.Int("count", len(orders)))

			for _, order := range orders {
				logger.Info("processing order with id ...", zap.Uint("id", order.ID), zap.Uint("status", uint(order.Status)))
				if strings.EqualFold(order.Maker, makerOrTaker.Hex()) {
					if order.Status == model.OrderFilled {
						logger.Info("initiator initiating an order", zap.Uint32("account", account), zap.Uint("order", order.ID))
						if err := handleInitiatorInitiateOrder(order, entropy, account, config, store, logger); err != nil {
							logger.Error("initiator failed to initiate the order:", zap.Error(err))
							continue
						}
					}

					if order.Status == model.FollowerAtomicSwapInitiated {
						secret, err := store.Secret(order.SecretHash)
						if err != nil {
							logger.Error("failed to retrieve the secret from db: ", zap.Error(err))
							continue
						}
						secretBytes, err := hex.DecodeString(secret)
						if err != nil {
							logger.Error("failed to decode the secret from db: ", zap.Error(err))
							continue
						}
						if err := handleInitiatorRedeemOrder(order, entropy, account, config, store, secretBytes, logger); err != nil {
							logger.Error("initiator failed to redeem the order:", zap.Error(err))
							continue
						}
					}

					if order.Status == model.InitiatorAtomicSwapInitiated || order.Status == model.FollowerAtomicSwapRefunded {
						// assuming that the function would just return nil if the swap has not expired yet
						if err := handleInitiatorRefund(order, entropy, account, config, store, logger); err != nil {
							logger.Info("initiator failed to refund the order:", zap.Error(err))
							continue
						}
					}
				}

				if strings.EqualFold(order.Taker, makerOrTaker.Hex()) {
					if order.Status == model.InitiatorAtomicSwapInitiated {
						if err := handleFollowerInitiateOrder(order, entropy, account, config, store, logger); err != nil {
							logger.Info("follower failed to initiate the order", zap.Error(err))
							continue
						}
					}

					if order.Status == model.FollowerAtomicSwapRedeemed {
						if err := handleFollowerRedeemOrder(order, entropy, account, config, store, logger); err != nil {
							logger.Info("follower failed to redeem the order", zap.Error(err))
							continue
						}
					}

					if order.Status == model.FollowerAtomicSwapInitiated {
						// assuming that the function would just return nil if the swap has not expired yet
						if err := handleFollowerRefund(order, entropy, account, config, store, logger); err != nil {
							logger.Info("follower failed to refund the order", zap.Error(err))
							continue
						}
					}
				}
			}
		}
	}
}

func handleInitiatorInitiateOrder(order model.Order, entropy []byte, user uint32, config model.Config, store Store, logger *zap.Logger) error {
	if isValid, err := store.CheckStatus(order.SecretHash); !isValid {
		logger.Info("skipping initiator initiate as it failed earlier", zap.Uint("order id", order.ID), zap.Error(errors.New(err)))
		return nil
	}

	status := store.Status(order.SecretHash)
	if status == InitiatorInitiated {
		return nil
	}

	fromChain, _, _, _, err := model.ParseOrderPair(order.OrderPair)
	if err != nil {
		return err
	}
	key, err := LoadKey(entropy, fromChain, user, 0)
	if err != nil {
		return err
	}
	keyInterface, err := key.Interface(order.InitiatorAtomicSwap.Chain)
	if err != nil {
		return err
	}

	initiatorSwap, err := blockchain.LoadInitiatorSwap(*order.InitiatorAtomicSwap, keyInterface, order.SecretHash, config.RPC, uint64(0))
	if err != nil {
		return err
	}
	txHash, err := initiatorSwap.Initiate()
	if err != nil {
		store.PutError(order.SecretHash, err.Error(), InitiatorFailedToInitiate)
		return err
	}
	if err := store.PutStatus(order.SecretHash, InitiatorInitiated); err != nil {
		return err
	}
	logger.Info("initiator initiated swap", zap.String("tx hash", txHash))
	return nil
}

func handleInitiatorRedeemOrder(order model.Order, entropy []byte, user uint32, config model.Config, store Store, secret []byte, logger *zap.Logger) error {

	if isValid, err := store.CheckStatus(order.SecretHash); !isValid {
		// if the bot is a initiator and redeem failed and bob did not refund
		if !strings.Contains(err, "Order not found in local storage") {
			if err := handleInitiatorRefund(order, entropy, user, config, store, logger); err != nil {
				return err
			}
		}
		logger.Info("skipping initiator redeem as it failed earlier", zap.Uint("order id", order.ID), zap.Error(errors.New(err)))
		return nil
	}

	status := store.Status(order.SecretHash)
	if status == InitiatorRedeemed {
		return nil
	}

	_, toChain, _, _, err := model.ParseOrderPair(order.OrderPair)
	if err != nil {
		return err
	}
	key, err := LoadKey(entropy, toChain, user, 0)
	if err != nil {
		return err
	}
	keyInterface, err := key.Interface(order.FollowerAtomicSwap.Chain)
	if err != nil {
		return err
	}

	redeemerSwap, err := blockchain.LoadRedeemerSwap(*order.FollowerAtomicSwap, keyInterface, order.SecretHash, config.RPC, uint64(0))

	if err != nil {
		return err
	}
	txHash, err := redeemerSwap.Redeem(secret)
	if err != nil {
		store.PutError(order.SecretHash, err.Error(), InitiatorFailedToRedeem)
		return err
	}

	if err := store.PutStatus(order.SecretHash, InitiatorRedeemed); err != nil {
		return err
	}
	logger.Info("initiator redeemed swap", zap.String("tx hash", txHash))
	return nil
}

func handleFollowerInitiateOrder(order model.Order, entropy []byte, user uint32, config model.Config, store Store, logger *zap.Logger) error {
	if isValid, err := store.CheckStatus(order.SecretHash); !isValid {
		logger.Info("skipping follower initiate as it failed earlier", zap.Uint("order id", order.ID), zap.Error(errors.New(err)))
		return nil
	}

	status := store.Status(order.SecretHash)
	if status == FollowerInitiated {
		return nil
	}

	_, toChain, _, _, err := model.ParseOrderPair(order.OrderPair)
	if err != nil {
		return err
	}
	key, err := LoadKey(entropy, toChain, user, 0)
	if err != nil {
		return err
	}
	keyInterface, err := key.Interface(order.FollowerAtomicSwap.Chain)
	if err != nil {
		return err
	}

	initiatorSwap, err := blockchain.LoadInitiatorSwap(*order.FollowerAtomicSwap, keyInterface, order.SecretHash, config.RPC, uint64(0))

	if err != nil {
		return err
	}
	txHash, err := initiatorSwap.Initiate()
	if err != nil {
		store.PutError(order.SecretHash, err.Error(), FollowerFailedToInitiate)
		return err
	}
	if err := store.PutStatus(order.SecretHash, FollowerInitiated); err != nil {
		return err
	}
	logger.Info("follower initiated swap", zap.String("tx hash", txHash))
	return nil
}

func handleFollowerRedeemOrder(order model.Order, entropy []byte, user uint32, config model.Config, store Store, logger *zap.Logger) error {
	if isValid, err := store.CheckStatus(order.SecretHash); !isValid {
		logger.Info("skipping follower redeem as it failed earlier", zap.Uint("order id", order.ID), zap.Error(errors.New(err)))
		return nil
	}

	status := store.Status(order.SecretHash)
	if status == FollowerRedeemed {
		return nil
	}

	fromChain, _, _, _, err := model.ParseOrderPair(order.OrderPair)
	if err != nil {
		return err
	}
	key, err := LoadKey(entropy, fromChain, user, 0)
	if err != nil {
		return err
	}
	keyInterface, err := key.Interface(order.InitiatorAtomicSwap.Chain)
	if err != nil {
		return err
	}

	redeemerSwap, err := blockchain.LoadRedeemerSwap(*order.InitiatorAtomicSwap, keyInterface, order.SecretHash, config.RPC, uint64(0))

	if err != nil {
		return err
	}

	secret, err := hex.DecodeString(order.Secret)
	if err != nil {
		return err
	}

	txHash, err := redeemerSwap.Redeem(secret)
	if err != nil {
		store.PutError(order.SecretHash, err.Error(), FollowerFailedToRedeem)
		return err
	}
	if err := store.PutStatus(order.SecretHash, FollowerRedeemed); err != nil {
		return err
	}
	logger.Info("follower redeemed swap", zap.String("tx hash", txHash))
	return nil
}
func handleFollowerRefund(order model.Order, entropy []byte, user uint32, config model.Config, store Store, logger *zap.Logger) error {
	status := store.Status(order.SecretHash)
	if status == FollowerRefunded {
		return nil
	}

	if isValid, err := store.CheckStatus(order.SecretHash); !isValid {
		logger.Info("skipping follower refund as it failed earlier", zap.Uint("order id", order.ID), zap.Error(errors.New(err)))
		return nil
	}
	_, toChain, _, _, err := model.ParseOrderPair(order.OrderPair)
	if err != nil {
		return err
	}
	key, err := LoadKey(entropy, toChain, user, 0)
	if err != nil {
		return err
	}
	keyInterface, err := key.Interface(order.FollowerAtomicSwap.Chain)
	if err != nil {
		return err
	}

	initiatorSwap, err := blockchain.LoadInitiatorSwap(*order.FollowerAtomicSwap, keyInterface, order.SecretHash, config.RPC, uint64(0))
	if err != nil {
		return err
	}
	isExpired, err := initiatorSwap.Expired()
	if err != nil {
		return err
	}

	if isExpired {
		txHash, err := initiatorSwap.Refund()
		if err != nil {
			store.PutError(order.SecretHash, err.Error(), FollowerFailedToRedeem)
			return err
		}
		if err := store.PutStatus(order.SecretHash, FollowerRefunded); err != nil {
			return err
		}
		logger.Info("follower refunded swap", zap.String("tx hash", txHash))
	}

	return nil
}
func handleInitiatorRefund(order model.Order, entropy []byte, user uint32, config model.Config, store Store, logger *zap.Logger) error {

	status := store.Status(order.SecretHash)
	if status == InitiatorRefunded {
		return nil
	}

	if isValid, err := store.CheckStatus(order.SecretHash); !isValid {
		logger.Info("skipping initiator refund as it failed earlier", zap.Uint("order id", order.ID), zap.Error(errors.New(err)))
		return nil
	}

	fromChain, _, _, _, err := model.ParseOrderPair(order.OrderPair)
	if err != nil {
		return err
	}
	key, err := LoadKey(entropy, fromChain, user, 0)
	if err != nil {
		return err
	}
	keyInterface, err := key.Interface(order.InitiatorAtomicSwap.Chain)
	if err != nil {
		return err
	}

	initiatorSwap, err := blockchain.LoadInitiatorSwap(*order.InitiatorAtomicSwap, keyInterface, order.SecretHash, config.RPC, uint64(0))
	if err != nil {
		return err
	}
	isExpired, err := initiatorSwap.Expired()
	if err != nil {
		return err
	}

	if isExpired {
		txHash, err := initiatorSwap.Refund()
		if err != nil {
			store.PutError(order.SecretHash, err.Error(), FollowerFailedToRedeem)
			return err
		}
		if err := store.PutStatus(order.SecretHash, InitiatorRefunded); err != nil {
			return err
		}
		logger.Info("initiator refunded swap", zap.String("tx hash", txHash))
	}

	return nil
}
