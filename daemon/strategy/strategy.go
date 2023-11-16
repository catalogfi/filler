package strategy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"github.com/catalogfi/guardian"
	"github.com/catalogfi/guardian/jsonrpc"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type strategy struct {
	config types.CoreConfig
	Quit   chan struct{}
	Wg     *sync.WaitGroup
}

type Stategy interface {
	RunAutoCreateStrategy(s AutoCreateStrategy, isIw bool)
	RunAutoFillStrategy(s AutoFillStrategy, isIw bool)
	Done()
}

func NewStrategy(config types.CoreConfig, wg *sync.WaitGroup) Stategy {
	quit := make(chan struct{})
	return &strategy{
		config: config,
		Quit:   quit,
		Wg:     wg,
	}
}

func (s *strategy) Done() {
	s.Quit <- struct{}{}
}

func (ac *strategy) RunAutoCreateStrategy(s AutoCreateStrategy, isIw bool) {
	defer func() {
		ac.config.Logger.Info("exiting auto create strategy")
		ac.Wg.Done()

	}()

	signer, client, err := utils.LoadClient(ac.config.EnvConfig.OrderBook, *ac.config.Keys, ac.config.Storage, s.account, 0)
	if err != nil {
		ac.config.Logger.Error("failed to connect to the client", zap.Error(err))
		return
	}

	fromChain, toChain, fromAsset, _, err := model.ParseOrderPair(s.OrderPair())
	if err != nil {
		ac.config.Logger.Error("failed while parsing order pair", zap.Error(err))
		return
	}

	fromKey, err := ac.config.Keys.GetKey(fromChain, s.account, 0)
	if err != nil {
		ac.config.Logger.Error("failed while getting from key", zap.Error(err))
		return
	}

	fromKeyInterface, err := fromKey.Interface(fromChain)
	if err != nil {
		ac.config.Logger.Error("failed to load sender key", zap.Error(err))
		return
	}

	var iwConfig []bitcoin.InstantWalletConfig

	if fromChain.IsBTC() && isIw {
		var iwStore bitcoin.Store
		if ac.config.EnvConfig.DB != "" {
			iwStore, err = bitcoin.NewStore(sqlite.Open(ac.config.EnvConfig.DB), &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
			})
			if err != nil {
				ac.config.Logger.Error("Could not load iw store: %v", zap.Error(err))
			}
		} else {
			iwStore, err = bitcoin.NewStore((utils.DefaultInstantWalletDBDialector()), &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
			})
			if err != nil {
				ac.config.Logger.Error("Could not load iw store: %v", zap.Error(err))
			}
		}
		privKey := fromKeyInterface.(*btcec.PrivateKey)
		chainParams := blockchain.GetParams(fromChain)
		rpcClient := jsonrpc.NewClient(new(http.Client), ac.config.EnvConfig.Network[fromChain].IWRPC)
		feeEstimator := btc.NewBlockstreamFeeEstimator(chainParams, ac.config.EnvConfig.Network[fromChain].RPC["mempool"], 20*time.Second)
		indexer := btc.NewElectrsIndexerClient(ac.config.Logger, ac.config.EnvConfig.Network[fromChain].RPC["mempool"], 5*time.Second)

		guardianWallet, err := guardian.NewBitcoinWallet(ac.config.Logger, privKey, chainParams, indexer, feeEstimator, rpcClient)
		if err != nil {
			ac.config.Logger.Error("failed to load gurdian wallet", zap.Error(err))
			return
		}
		iwConfig = append(iwConfig, bitcoin.InstantWalletConfig{
			Store:   iwStore,
			IWallet: guardianWallet,
		})
	}

	// Get the addresses on different chains.

	iWfromAddress, err := fromKey.Address(fromChain, ac.config.EnvConfig.Network, false, iwConfig...)
	if err != nil {
		ac.config.Logger.Error("failed while getting address string", zap.Error(err))
		return
	}

	fromAddress, err := fromKey.Address(fromChain, ac.config.EnvConfig.Network, false)
	if err != nil {
		ac.config.Logger.Error("failed while getting address string", zap.Error(err))
		return
	}
	toKey, err := ac.config.Keys.GetKey(toChain, s.account, 0)
	if err != nil {
		ac.config.Logger.Error("failed while getting to key", zap.Error(err))
		return
	}
	toAddress, err := toKey.Address(toChain, ac.config.EnvConfig.Network, false)
	if err != nil {
		ac.config.Logger.Error("failed while getting address string", zap.Error(err))
		return
	}

	randTimeInterval, err := rand.Int(rand.Reader, big.NewInt(int64(s.MaxTimeInterval-s.MinTimeInterval)))
	if err != nil {
		ac.config.Logger.Error("can't create a random time", zap.Error(err))
		return
	}
	randTimeInterval.Add(randTimeInterval, big.NewInt(int64(s.MinTimeInterval)))

	for {
		select {
		case <-time.After(time.Duration(randTimeInterval.Int64()) * time.Second):
			{
				randTimeInterval, err = rand.Int(rand.Reader, big.NewInt(int64(s.MaxTimeInterval-s.MinTimeInterval)))
				if err != nil {
					ac.config.Logger.Error("can't create a random time", zap.Error(err))
					return
				}
				randTimeInterval.Add(randTimeInterval, big.NewInt(int64(s.MinTimeInterval)))

				randAmount, err := rand.Int(rand.Reader, new(big.Int).Sub(s.maxAmount, s.minAmount))
				if err != nil {
					ac.config.Logger.Error("can't create a random amount", zap.Error(err))
					return
				}
				randAmount.Add(randAmount, s.minAmount)

				balance, err := utils.VirtualBalance(fromChain, iWfromAddress, ac.config.EnvConfig.Network, fromAsset, signer.Hex(), client, iwConfig...)
				if err != nil {
					ac.config.Logger.Error("failed to get virtual balance", zap.String("address", iWfromAddress), zap.Error(err))
					return
				}

				if balance.Cmp(randAmount) < 0 {
					ac.config.Logger.Info("insufficient balance", zap.String("chain", string(fromChain)), zap.String("asset", string(fromAsset)), zap.String("address", fromAddress), zap.String("have", balance.String()), zap.String("need", randAmount.String()))
					continue
				}

				receiveAmount, err := s.priceStrategy.CalculatereceiveAmount(randAmount)
				if err != nil {
					ac.config.Logger.Error("failed while getting address string", zap.Error(err))
					return
				}

				secret := [32]byte{}
				_, err = rand.Read(secret[:])
				if err != nil {
					ac.config.Logger.Error("failed to read secret", zap.Error(err))
					return
				}
				secretHash := sha256.Sum256(secret[:])

				id, err := client.CreateOrder(fromAddress, toAddress, s.orderPair, randAmount.String(), receiveAmount.String(), hex.EncodeToString(secretHash[:]))
				if err != nil {
					ac.config.Logger.Error("failed while creating order", zap.Error(err))
					return
				}

				if err := ac.config.Storage.UserStore(s.account).PutSecret(hex.EncodeToString(secretHash[:]), hex.EncodeToString(secret[:]), uint64(id)); err != nil {
					ac.config.Logger.Error("failed to store secret", zap.Error(err))
					return
				}
				continue

			}

		case <-ac.Quit:
			{
				ac.config.Logger.Info("terminating with auto creator")
				break
			}
		}
	}
}

func (af *strategy) RunAutoFillStrategy(s AutoFillStrategy, isIw bool) {
	defer func() {
		af.config.Logger.Info("exiting auto fill strategy")
		af.Wg.Done()
	}()

	// Load keys
	signer, client, err := utils.LoadClient(af.config.EnvConfig.OrderBook, *af.config.Keys, af.config.Storage, s.account, 0)
	if err != nil {
		af.config.Logger.Error("can't load the client", zap.Error(err))
		return
	}

	for {
		select {
		case <-time.After(10 * time.Second):
			{

				price, err := s.PriceStrategy().Price()
				if err != nil {
					af.config.Logger.Error("failed calculating price", zap.Error(err))
					continue
				}

				orders, err := client.GetOrders(rest.GetOrdersFilter{
					Maker:     strings.Join(s.Makers, ","),
					OrderPair: s.OrderPair(),
					MinPrice:  price,
					MaxPrice:  math.MaxFloat64,
					Status:    int(model.Created),
					Verbose:   true,
				})
				if err != nil {
					af.config.Logger.Error("failed parsing order pair", zap.Error(err), zap.Any("filter", rest.GetOrdersFilter{
						Maker:     strings.Join(s.Makers, ","),
						OrderPair: s.OrderPair(),
						MinPrice:  price,
						MaxPrice:  math.MaxFloat64,
						Status:    int(model.Created),
						Verbose:   true,
					}))
					continue
				}

				toChain, fromChain, _, fromAsset, err := model.ParseOrderPair(s.OrderPair())
				if err != nil {
					af.config.Logger.Error("failed parsing order pair", zap.Error(err))
					return
				}

				// Get the addresses on different chains.
				fromKey, err := af.config.Keys.GetKey(fromChain, s.account, 0)
				if err != nil {
					af.config.Logger.Error("failed getting from key", zap.Error(err))
					return
				}
				fromKeyInterface, err := fromKey.Interface(fromChain)
				if err != nil {
					af.config.Logger.Error("failed to load sender key", zap.Error(err))
					return
				}

				var iwConfig []bitcoin.InstantWalletConfig

				if fromChain.IsBTC() && isIw {
					var iwStore bitcoin.Store
					if af.config.EnvConfig.DB != "" {
						iwStore, err = bitcoin.NewStore(sqlite.Open(af.config.EnvConfig.DB), &gorm.Config{
							NowFunc: func() time.Time { return time.Now().UTC() },
						})
						if err != nil {
							af.config.Logger.Error("Could not load iw store: %v", zap.Error(err))
						}
					} else {
						iwStore, err = bitcoin.NewStore((utils.DefaultInstantWalletDBDialector()), &gorm.Config{
							NowFunc: func() time.Time { return time.Now().UTC() },
						})
						if err != nil {
							af.config.Logger.Error("Could not load iw store: %v", zap.Error(err))
						}
					}
					privKey := fromKeyInterface.(*btcec.PrivateKey)
					chainParams := blockchain.GetParams(fromChain)
					rpcClient := jsonrpc.NewClient(new(http.Client), af.config.EnvConfig.Network[fromChain].IWRPC)
					feeEstimator := btc.NewBlockstreamFeeEstimator(chainParams, af.config.EnvConfig.Network[fromChain].RPC["mempool"], 20*time.Second)
					indexer := btc.NewElectrsIndexerClient(af.config.Logger, af.config.EnvConfig.Network[fromChain].RPC["mempool"], 5*time.Second)

					guardianWallet, err := guardian.NewBitcoinWallet(af.config.Logger, privKey, chainParams, indexer, feeEstimator, rpcClient)
					if err != nil {
						af.config.Logger.Error("failed to load gurdian wallet", zap.Error(err))
						return
					}
					iwConfig = append(iwConfig, bitcoin.InstantWalletConfig{
						Store:   iwStore,
						IWallet: guardianWallet,
					})
				}
				iWfromAddress, err := fromKey.Address(fromChain, af.config.EnvConfig.Network, false, iwConfig...)
				if err != nil {
					af.config.Logger.Error("failed while getting address string", zap.Error(err))
					return
				}
				fromAddress, err := fromKey.Address(fromChain, af.config.EnvConfig.Network, false)
				if err != nil {
					af.config.Logger.Error("failed getting from address string", zap.Error(err))
					return
				}
				toKey, err := af.config.Keys.GetKey(toChain, s.account, 0)
				if err != nil {
					af.config.Logger.Error("failed getting to key", zap.Error(err))
					return
				}
				toAddress, err := toKey.Address(toChain, af.config.EnvConfig.Network, false)
				if err != nil {
					af.config.Logger.Error("failed getting to address string", zap.Error(err))
					return
				}

				for _, order := range orders {

					balance, err := utils.VirtualBalance(fromChain, iWfromAddress, af.config.EnvConfig.Network, fromAsset, signer.Hex(), client, iwConfig...)
					if err != nil {
						af.config.Logger.Error("failed to get virtual balance", zap.String("address", fromAddress), zap.Error(err))
						continue
					}

					if order.FollowerAtomicSwap == nil {
						af.config.Logger.Error("malformed order", zap.Any("order", order))
						continue
					}

					orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
					if !ok {
						af.config.Logger.Info("failed to get order amount", zap.Error(err))
						continue
					}

					if balance.Cmp(orderAmount) < 0 {
						af.config.Logger.Info("insufficient balance", zap.String("chain", string(fromChain)), zap.String("asset", string(fromAsset)), zap.String("address", iWfromAddress), zap.String("have", balance.String()), zap.String("need", orderAmount.String()))
						continue
					}

					if err := client.FillOrder(order.ID, fromAddress, toAddress); err != nil {
						af.config.Logger.Error("failed to fill the order ❌", zap.Uint("id", order.ID), zap.Error(err))
						continue
					}

					if err = af.config.Storage.UserStore(s.account).PutSecretHash(order.SecretHash, uint64(order.ID)); err != nil {
						af.config.Logger.Error("failed storing secret hash: %v", zap.Error(err))
						continue
					}
					af.config.Logger.Info("filled order ✅", zap.Uint("id", order.ID))
				}
			}
		case <-af.Quit:
			{
				af.config.Logger.Info("terminating with auto creator")
				break
			}
		}
	}
}
