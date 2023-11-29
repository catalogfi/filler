package strategy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/pkg/swapper/bitcoin"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"go.uber.org/zap"
)

type StrategyType string

const (
	Filler  StrategyType = "fill"
	Creator StrategyType = "create"
)

type strategy struct {
	config types.CoreConfig
	Quit   chan struct{}
	Wg     *sync.WaitGroup
}

type StrategyService interface {
	RunAutoCreateStrategy(s AutoCreateStrategy)
	RunAutoFillStrategy(s AutoFillStrategy)
	Done()
}

func NewStrategyService(config types.CoreConfig, wg *sync.WaitGroup) StrategyService {
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

func (ac *strategy) RunAutoCreateStrategy(s AutoCreateStrategy) {
	defer func() {
		ac.config.Logger.Info("exiting auto create strategy")
		ac.Wg.Done()

	}()

	ac.config.Logger.Info("starting autocreator")

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

	if fromChain.IsBTC() && s.UseIw {
		iwStore, err := utils.LoadIwDB(ac.config.EnvConfig.DB)
		if err != nil {
			ac.config.Logger.Info("Could not load iw store: %v", zap.Error(err))
		}

		guardianWallet, err := utils.GetGuardianWallet(fromKeyInterface, ac.config.Logger, fromChain, ac.config.EnvConfig.Network)
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

				receiveAmount, err := s.priceStrategy.CalculateReceiveAmount(randAmount)
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
				ac.config.Logger.Info("terminating auto creator")
				break
			}
		}
	}
}

func (af *strategy) RunAutoFillStrategy(s AutoFillStrategy) {
	defer func() {
		af.config.Logger.Info("exiting auto fill strategy")
		af.Wg.Done()
	}()

	af.config.Logger.Info("starting autofiller")

	// Load keys
	signer, client, err := utils.LoadClient(af.config.EnvConfig.OrderBook, *af.config.Keys, af.config.Storage, s.account, 0)
	if err != nil {
		af.config.Logger.Error("can't load the client", zap.Error(err))
		return
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

	if fromChain.IsBTC() && s.UseIw {
		iwStore, err := utils.LoadIwDB(af.config.EnvConfig.DB)
		if err != nil {
			af.config.Logger.Info("Could not load iw store: %v", zap.Error(err))
		}

		guardianWallet, err := utils.GetGuardianWallet(fromKeyInterface, af.config.Logger, fromChain, af.config.EnvConfig.Network)
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

	price, err := s.PriceStrategy().Price()
	if err != nil {
		af.config.Logger.Error("failed calculating price", zap.Error(err))
		return
	}

	// exponetial set back is used to reconnect to the socket in cal of a error
	expSb := time.Second
	for {
		// connect to the websocket and subscribe on the signer's address
		wsClient := rest.NewWSClient(fmt.Sprintf("wss://%s/", af.config.EnvConfig.OrderBook), af.config.Logger)
		wsClient.Subscribe(fmt.Sprintf("subscribe::%v", s.orderPair))
		respChan := wsClient.Listen()
	SIGNALOOP:
		for {
			select {
			case resp := <-respChan:
				switch response := resp.(type) {
				case rest.WebsocketError:
					break SIGNALOOP
				case rest.OpenOrder:
					{
						expSb = time.Second
						order := response.Order

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

						if order.Price < price {
							// todo : add other price strategies
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
					af.config.Logger.Info("terminating auto filler")
					return
				}

			}
		}
		time.Sleep(expSb)
		if expSb < (8 * time.Second) {
			expSb *= 2
		}
	}
}

func Uid(strat Strategy) (string, error) {
	hash, err := utils.HashData(strat)
	if err != nil {
		return "", nil
	}
	return strings.Join([]string{hash[:8], strat.StrategyType}, "_"), nil
}
