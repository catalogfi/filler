package executor

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/pkg/blockchain"
	"github.com/catalogfi/cobi/pkg/swapper/bitcoin"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
)

type executor struct {
	Config types.CoreConfig
	Quit   chan struct{}
	Wg     *sync.WaitGroup
}

type RequestStartExecutor struct {
	Account         uint32 `json:"userAccount"`
	IsInstantWallet bool   `json:"isInstantWallet"`
}

type AccountExecutor interface {
	Start(params RequestStartExecutor)
	Done()
}

func NewExecutor(config types.CoreConfig, wg *sync.WaitGroup) AccountExecutor {
	quit := make(chan struct{})
	return &executor{
		Config: config,
		Quit:   quit,
		Wg:     wg,
	}
}

func (e *executor) Done() {
	e.Quit <- struct{}{}
}

func (e *executor) Start(params RequestStartExecutor) {

	defer func() {
		e.Config.Logger.Info("exiting executor")
		e.Wg.Done()
	}()

	key, err := e.Config.Keys.GetKey(model.Ethereum, params.Account, 0)
	if err != nil {
		e.Config.Logger.Error("failed to get the signing key:", zap.Error(err))
		return
	}

	var iwConfig []bitcoin.InstantWalletConfig

	if params.IsInstantWallet {
		iwStore, err := utils.LoadIwDB(e.Config.EnvConfig.DB)
		if err != nil {
			e.Config.Logger.Info("Could not load iw store: %v", zap.Error(err))
		}
		iwConfig = append(iwConfig, bitcoin.InstantWalletConfig{
			Store: iwStore,
		})

	}

	privKey, err := key.ECDSA()
	if err != nil {
		e.Config.Logger.Error("failed to get the signing key:", zap.Error(err))
		return
	}
	signer := crypto.PubkeyToAddress(privKey.PublicKey)

	expSb := time.Second
LOOP:
	for {
		e.Config.Logger.Info("subcribing to socket")
		// connect to the websocket and subscribe on the signer's address
		client := rest.NewWSClient(fmt.Sprintf("wss://%s/", e.Config.EnvConfig.OrderBook), e.Config.Logger)
		client.Subscribe(fmt.Sprintf("subscribe::%v", signer))
		respChan := client.Listen()
	SIGNALOOP:
		for {

			select {
			case resp, ok := <-respChan:
				if !ok {
					break SIGNALOOP
				}
				expSb = time.Second
				switch response := resp.(type) {
				case rest.WebsocketError:
					break SIGNALOOP
				case rest.UpdatedOrders:
					// execute orders
					orders := response.Orders
					count := len(orders)
					e.Config.Logger.Info("recieved orders from the order book", zap.Int("count", count))
					for _, order := range orders {
						grandChildLogger := e.Config.Logger.With(zap.Uint("order id", order.ID), zap.String("pair", order.OrderPair))
						Execute(order, grandChildLogger, signer, *e.Config.Keys, params.Account, e.Config.EnvConfig.Network, e.Config.Storage.UserStore(params.Account), iwConfig...)
					}
					e.Config.Logger.Info("executed orders recieved from the order book", zap.Int("count", count))
				}
				continue
			case <-e.Quit:
				e.Config.Logger.Info("recieved quit channel signal")
				break LOOP
			}

		}
		time.Sleep(expSb)
		if expSb < (8 * time.Second) {
			expSb *= 2
		}
	}
}

func Execute(order model.Order, logger *zap.Logger, signer common.Address, keys utils.Keys, account uint32, config model.Network, userStore store.UserStore, iwConfig ...bitcoin.InstantWalletConfig) {
	logger.Info("processing order with id", zap.Uint("status", uint(order.Status)))
	if isValid, err := userStore.CheckStatus(order.SecretHash); !isValid {
		if err != "" {
			logger.Error("failed to check status", zap.Error(errors.New(err)))
		} else {
			logger.Info("skipping order as it failed earlier")
		}
		return
	}

	logger.Info("processing order with id", zap.Uint("status", uint(order.Status)))

	if isValid, err := userStore.CheckStatus(order.SecretHash); err != "" {
		if isValid {
			logger.Info("skipping order as it failed earlier", zap.Error(errors.New(err)))
		} else {
			logger.Error("failed to load a swap from the db", zap.Error(errors.New(err)))
			return
		}
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
			if order.FollowerAtomicSwap.Status == model.Detected {
				logger.Info("detected follower atomic swap", zap.String("txHash", order.FollowerAtomicSwap.InitiateTxHash))
			} else if status != store.InitiatorInitiated && status != store.InitiatorFailedToInitiate && order.FollowerAtomicSwap.Status == model.SwapStatus(model.Unknown) {
				handleInitiate(*order.InitiatorAtomicSwap, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator initiate")), true, iwConfig...)
			} else if order.FollowerAtomicSwap.Status == model.Initiated {
				if status != store.InitiatorRedeemed && status != store.InitiatorFailedToRedeem {
					secret, err := userStore.Secret(order.SecretHash)
					if err != nil {
						logger.Error("failed to retrieve the secret from db", zap.Error(err))
						return
					}
					handleRedeem(*order.FollowerAtomicSwap, secret, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator redeem")), true, iwConfig...)
				}
			} else if order.InitiatorAtomicSwap.Status == model.Expired {
				if status == store.InitiatorInitiated {
					// assuming that the function would just return nil if the swap has not expired yet
					handleRefund(*order.InitiatorAtomicSwap, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator refund")), true, iwConfig...)
				}
			}
		}
	} else if strings.EqualFold(order.Taker, signer.Hex()) {
		if order.Status == model.Filled {
			if order.InitiatorAtomicSwap.Status == model.Detected {
				logger.Info("detected initiator atomic swap", zap.String("txHash", order.InitiatorAtomicSwap.InitiateTxHash))
			} else if order.FollowerAtomicSwap.Status == model.Redeemed && order.InitiatorAtomicSwap.Status == model.Initiated {
				if status != store.FollowerRedeemed && status != store.FollowerFailedToRedeem {
					handleRedeem(*order.InitiatorAtomicSwap, order.FollowerAtomicSwap.Secret, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "follower redeem")), false, iwConfig...)
				}
			} else if order.InitiatorAtomicSwap.Status == model.Initiated {
				if status != store.FollowerInitiated && status != store.FollowerFailedToInitiate {
					handleInitiate(*order.FollowerAtomicSwap, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "follower initiate")), false, iwConfig...)
				}
			} else if order.FollowerAtomicSwap.Status == model.Expired {
				// assuming that the function would just return nil if the swap has not expired yet
				if status == store.FollowerInitiated {
					handleRefund(*order.FollowerAtomicSwap, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "follower refund")), false, iwConfig...)
				}
			}
		}
	}
}

func handleRedeem(atomicSwap model.AtomicSwap, secret, secretHash string, keyInterface interface{}, config model.Network, userStore store.UserStore, logger *zap.Logger, isInitiator bool, iwConfig ...bitcoin.InstantWalletConfig) {
	logger.Info("redeeming an order")
	if len(iwConfig) != 0 && atomicSwap.Chain.IsBTC() {
		guardianWallet, err := utils.GetGuardianWallet(keyInterface, logger, atomicSwap.Chain, config)
		if err != nil {
			logger.Error("failed to load gurdian wallet", zap.Error(err))
			return
		}
		iwConfig[0].IWallet = guardianWallet
	}

	redeemerSwap, err := blockchain.LoadRedeemerSwap(atomicSwap, keyInterface, secretHash, config, uint64(0), iwConfig...)
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
	if err := userStore.PutTxHash(secretHash, store.Redeemed, txHash); err != nil {
		logger.Error("failed to update tx hash", zap.Error(err))
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

func handleInitiate(atomicSwap model.AtomicSwap, secretHash string, keyInterface interface{}, config model.Network, userStore store.UserStore, logger *zap.Logger, isInitiator bool, iwConfig ...bitcoin.InstantWalletConfig) {
	logger.Info("initiating an order")
	if len(iwConfig) != 0 && atomicSwap.Chain.IsBTC() {
		guardianWallet, err := utils.GetGuardianWallet(keyInterface, logger, atomicSwap.Chain, config)
		if err != nil {
			logger.Error("failed to load gurdian wallet", zap.Error(err))
			return
		}
		iwConfig[0].IWallet = guardianWallet
	}
	initiatorSwap, err := blockchain.LoadInitiatorSwap(atomicSwap, keyInterface, secretHash, config, uint64(0), iwConfig...)
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

	if err := userStore.PutTxHash(secretHash, store.Initated, txHash); err != nil {
		logger.Error("failed to update tx hash", zap.Error(err))
	}

	status := store.InitiatorInitiated
	if !isInitiator {
		status = store.FollowerInitiated
	}
	if err := userStore.PutStatus(secretHash, status); err != nil {
		logger.Error("failed to update status", zap.Error(err))
	}
}

func handleRefund(atomicSwap model.AtomicSwap, secretHash string, keyInterface interface{}, config model.Network, userStore store.UserStore, logger *zap.Logger, isInitiator bool, iwConfig ...bitcoin.InstantWalletConfig) {
	logger.Info("refunding an order")
	if len(iwConfig) != 0 && atomicSwap.Chain.IsBTC() {
		guardianWallet, err := utils.GetGuardianWallet(keyInterface, logger, atomicSwap.Chain, config)
		if err != nil {
			logger.Error("failed to load gurdian wallet", zap.Error(err))
			return
		}
		iwConfig[0].IWallet = guardianWallet
	}
	timelock, err := strconv.ParseUint(atomicSwap.Timelock, 10, 64)
	if err != nil {
		logger.Error("failed to parse timelock", zap.Error(err))
		return
	}
	initiatorSwap, err := blockchain.LoadInitiatorSwap(atomicSwap, keyInterface, secretHash, config, timelock, iwConfig...)
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
		if err := userStore.PutTxHash(secretHash, store.Refunded, txHash); err != nil {
			logger.Error("failed to update tx hash", zap.Error(err))
		}
		logger.Info("successfully refunded swap", zap.String("tx hash", txHash))

		status := store.InitiatorRefunded
		if !isInitiator {
			status = store.FollowerRefunded
		}
		if err := userStore.PutStatus(secretHash, status); err != nil {
			logger.Error("failed to update status", zap.Error(err))
		}
	} else {
		logger.Error("failed to refund status : swap not expired")
	}
}

func Uid(useIw bool, account uint32) (string, error) {
	hash, err := utils.HashData(fmt.Sprintf("executor_account_%d_%t", account, useIw))
	if err != nil {
		return "", nil
	}
	return strings.Join([]string{hash[:8], "exec"}, "_"), nil
}
