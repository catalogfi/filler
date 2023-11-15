package executor

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/cobid/types"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"github.com/catalogfi/guardian"
	"github.com/catalogfi/guardian/jsonrpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type executor struct{}

// type CoreConfig struct {
// 	Storage   store.Store
// 	EnvConfig utils.Config
// 	Keys      *utils.Keys
// 	Logger    *zap.Logger
// }

type RequestStartExecutor struct {
	Account         uint32 `json:"userAccount"`
	IsInstantWallet bool   `json:"isInstantWallet"`
}

type AccountExecutor interface {
	Start(cfg types.CoreConfig, params RequestStartExecutor)
}

func NewExecutor() AccountExecutor {
	return &executor{}
}

func (e *executor) Start(cfg types.CoreConfig, params RequestStartExecutor) {

	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(config)
	logFile, _ := os.OpenFile(filepath.Join(utils.DefaultCobiDirectory(), fmt.Sprintf("executor_account_%d.log", params.Account)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	writer := zapcore.AddSync(logFile)
	defaultLogLevel := zapcore.DebugLevel
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, defaultLogLevel),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	pidFilePath := filepath.Join(utils.DefaultCobiDirectory(), fmt.Sprintf("executor_account_%d.pid", params.Account))

	if _, err := os.Stat(pidFilePath); err == nil {
		panic("executor already running")
	}
	pid := strconv.Itoa(os.Getpid())
	err := os.WriteFile(pidFilePath, []byte(pid), 0644)
	if err != nil {
		panic("failed to write pid")
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGQUIT)

	key, err := cfg.Keys.GetKey(model.Ethereum, params.Account, 0)
	if err != nil {
		panic(fmt.Errorf("failed to get the signing key:", zap.Error(err)))
	}

	var iwConfig []bitcoin.InstantWalletConfig

	if params.IsInstantWallet {
		var iwStore bitcoin.Store
		if cfg.EnvConfig.DB != "" {
			iwStore, err = bitcoin.NewStore(sqlite.Open(cfg.EnvConfig.DB), &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
			})
			if err != nil {
				logger.Error("Could not load iw store: %v", zap.Error(err))
			}
		} else {
			iwStore, err = bitcoin.NewStore((utils.DefaultInstantWalletDBDialector()), &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
			})
			if err != nil {
				logger.Error("Could not load iw store: %v", zap.Error(err))
			}
		}
		iwConfig = append(iwConfig, bitcoin.InstantWalletConfig{
			Store: iwStore,
		})

	}

	privKey, err := key.ECDSA()
	if err != nil {
		panic(fmt.Errorf("failed to get the signing key:", zap.Error(err)))
	}
	signer := crypto.PubkeyToAddress(privKey.PublicKey)
LOOP:
	for {
		// connect to the websocket and subscribe on the signer's address
		client := rest.NewWSClient(fmt.Sprintf("wss://%s/", cfg.EnvConfig.OrderBook), logger)
		client.Subscribe(fmt.Sprintf("subscribe_%v", signer))
		respChan := client.Listen()
		for {

			select {
			case resp := <-respChan:
				switch response := resp.(type) {
				case rest.WebsocketError:
					break
				case rest.UpdatedOrders:
					// execute orders
					orders := response.Orders
					count := len(orders)
					logger.Info("recieved orders from the order book", zap.Int("count", count))
					for _, order := range orders {
						grandChildLogger := logger.With(zap.Uint("order id", order.ID), zap.String("pair", order.OrderPair))
						e.execute(order, grandChildLogger, signer, *cfg.Keys, params.Account, cfg.EnvConfig.Network, cfg.Storage.UserStore(params.Account), iwConfig...)
					}
					logger.Info("executed orders recieved from the order book", zap.Int("count", count))

				}
			case sig := <-sigs:
				if sig == syscall.SIGQUIT {

					if _, err := os.Stat(pidFilePath); err == nil {
						err := os.Remove(pidFilePath)
						if err != nil {
							logger.Error("failed to delete executor pid file", zap.Uint32("account", params.Account), zap.Error(err))
						}
					} else {
						logger.Error("executor pid file not found", zap.Uint32("account", params.Account), zap.Error(err))
					}
					logger.Info("stopped", zap.Uint32("account", params.Account))
					break LOOP
				}
			}
		}
	}
	logger.Info("terminated", zap.Uint32("account", params.Account))
}

func (e *executor) execute(order model.Order, logger *zap.Logger, signer common.Address, keys utils.Keys, account uint32, config model.Network, userStore store.UserStore, iwConfig ...bitcoin.InstantWalletConfig) {
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
				e.handleInitiate(*order.InitiatorAtomicSwap, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator initiate")), true, iwConfig...)
			} else if order.FollowerAtomicSwap.Status == model.Initiated {
				if status != store.InitiatorRedeemed && status != store.InitiatorFailedToRedeem {
					secret, err := userStore.Secret(order.SecretHash)
					if err != nil {
						logger.Error("failed to retrieve the secret from db", zap.Error(err))
						return
					}
					e.handleRedeem(*order.FollowerAtomicSwap, secret, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator redeem")), true, iwConfig...)
				}
			} else if order.InitiatorAtomicSwap.Status == model.Expired {
				if status == store.InitiatorInitiated {
					// assuming that the function would just return nil if the swap has not expired yet
					e.handleRefund(*order.InitiatorAtomicSwap, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "initiator refund")), true, iwConfig...)
				}
			}
		}
	} else if strings.EqualFold(order.Taker, signer.Hex()) {
		if order.Status == model.Filled {
			if order.InitiatorAtomicSwap.Status == model.Detected {
				logger.Info("detected initiator atomic swap", zap.String("txHash", order.InitiatorAtomicSwap.InitiateTxHash))
			} else if order.FollowerAtomicSwap.Status == model.Redeemed && order.InitiatorAtomicSwap.Status == model.Initiated {
				if status != store.FollowerRedeemed && status != store.FollowerFailedToRedeem {
					e.handleRedeem(*order.InitiatorAtomicSwap, order.FollowerAtomicSwap.Secret, order.SecretHash, fromKeyInterface, config, userStore, logger.With(zap.String("handler", "follower redeem")), false, iwConfig...)
				}
			} else if order.InitiatorAtomicSwap.Status == model.Initiated {
				if status != store.FollowerInitiated && status != store.FollowerFailedToInitiate {
					e.handleInitiate(*order.FollowerAtomicSwap, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "follower initiate")), false, iwConfig...)
				}
			} else if order.FollowerAtomicSwap.Status == model.Expired {
				// assuming that the function would just return nil if the swap has not expired yet
				if status == store.FollowerInitiated {
					e.handleRefund(*order.FollowerAtomicSwap, order.SecretHash, toKeyInterface, config, userStore, logger.With(zap.String("handler", "follower refund")), false, iwConfig...)
				}
			}
		}
	}
}

func (e *executor) handleRedeem(atomicSwap model.AtomicSwap, secret, secretHash string, keyInterface interface{}, config model.Network, userStore store.UserStore, logger *zap.Logger, isInitiator bool, iwConfig ...bitcoin.InstantWalletConfig) {
	logger.Info("redeeming an order")
	if len(iwConfig) != 0 && atomicSwap.Chain.IsBTC() {
		privKey := keyInterface.(*btcec.PrivateKey)
		chainParams := blockchain.GetParams(atomicSwap.Chain)
		rpcClient := jsonrpc.NewClient(new(http.Client), config[atomicSwap.Chain].IWRPC)
		feeEstimator := btc.NewBlockstreamFeeEstimator(chainParams, config[atomicSwap.Chain].RPC["mempool"], 20*time.Second)
		indexer := btc.NewElectrsIndexerClient(logger, config[atomicSwap.Chain].RPC["mempool"], 5*time.Second)

		guardianWallet, err := guardian.NewBitcoinWallet(logger, privKey, chainParams, indexer, feeEstimator, rpcClient)
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

func (e *executor) handleInitiate(atomicSwap model.AtomicSwap, secretHash string, keyInterface interface{}, config model.Network, userStore store.UserStore, logger *zap.Logger, isInitiator bool, iwConfig ...bitcoin.InstantWalletConfig) {
	logger.Info("initiating an order")
	if len(iwConfig) != 0 && atomicSwap.Chain.IsBTC() {
		privKey := keyInterface.(*btcec.PrivateKey)
		chainParams := blockchain.GetParams(atomicSwap.Chain)
		rpcClient := jsonrpc.NewClient(new(http.Client), config[atomicSwap.Chain].IWRPC)
		feeEstimator := btc.NewBlockstreamFeeEstimator(chainParams, config[atomicSwap.Chain].RPC["mempool"], 20*time.Second)
		indexer := btc.NewElectrsIndexerClient(logger, config[atomicSwap.Chain].RPC["mempool"], 5*time.Second)

		guardianWallet, err := guardian.NewBitcoinWallet(logger, privKey, chainParams, indexer, feeEstimator, rpcClient)
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

func (e *executor) handleRefund(atomicSwap model.AtomicSwap, secretHash string, keyInterface interface{}, config model.Network, userStore store.UserStore, logger *zap.Logger, isInitiator bool, iwConfig ...bitcoin.InstantWalletConfig) {
	logger.Info("refunding an order")
	if len(iwConfig) != 0 && atomicSwap.Chain.IsBTC() {
		privKey := keyInterface.(*btcec.PrivateKey)
		chainParams := blockchain.GetParams(atomicSwap.Chain)
		rpcClient := jsonrpc.NewClient(new(http.Client), config[atomicSwap.Chain].IWRPC)
		feeEstimator := btc.NewBlockstreamFeeEstimator(chainParams, config[atomicSwap.Chain].RPC["mempool"], 20*time.Second)
		indexer := btc.NewElectrsIndexerClient(logger, config[atomicSwap.Chain].RPC["mempool"], 5*time.Second)

		guardianWallet, err := guardian.NewBitcoinWallet(logger, privKey, chainParams, indexer, feeEstimator, rpcClient)
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
