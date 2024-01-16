package filler

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

type Filler interface {
	// Start the filler to match orders, it's not blocking and will spawn a background goroutine.
	Start() error

	// Stop will gracefully shut down the Filler, it waits for all inner goroutines to finish.
	Stop()
}

type filler struct {
	strategies Strategies
	btcWallet  btcswap.Wallet
	ethWallets map[model.Chain]ethswap.Wallet
	dialer     func() rest.WSClient
	restClient rest.Client
	logger     *zap.Logger
	quit       chan struct{}
	wg         *sync.WaitGroup
}

func New(strategies Strategies, btcWallet btcswap.Wallet, ethWallets map[model.Chain]ethswap.Wallet, restClient rest.Client, dialer func() rest.WSClient, logger *zap.Logger) Filler {
	return &filler{
		strategies: strategies,
		btcWallet:  btcWallet,
		ethWallets: ethWallets,
		dialer:     dialer,
		restClient: restClient,
		logger:     logger.With(zap.String("component", "filler")),
		quit:       make(chan struct{}),
		wg:         new(sync.WaitGroup),
	}
}

func (f *filler) Stop() {
	if f.quit != nil {
		close(f.quit)
		f.wg.Wait()
		f.quit = nil
	}
}

func (f *filler) Start() error {
	for _, strategy := range f.strategies {
		orderPair := strategy.OrderPair
		f.wg.Add(1)

		go func(orderPair string, strategy Strategy) {
			defer f.wg.Done()

			for {
				f.logger.Info("subscribing to orderPair", zap.String("orderPair", orderPair))
				client := f.dialer()
				client.Subscribe(fmt.Sprintf("subscribe::%v", orderPair))
				respChan := client.Listen()

			Orders:
				for {
					select {
					case resp, ok := <-respChan:
						if !ok {
							break Orders
						}

						switch response := resp.(type) {
						case rest.WebsocketError:
							break Orders
						case rest.OpenOrders:
							orders := response.Orders
							for _, order := range orders {
								filled, err := f.fill(strategy, order)
								if err != nil {
									f.logger.Error("❌ [FILL]", zap.Uint("id", order.ID), zap.Error(err))
									continue
								}

								if filled {
									f.logger.Info("✅ [FILL]", zap.Uint("id", order.ID))
								} else {
									f.logger.Info("❌ [FILL] order not match our strategy", zap.Uint("id", order.ID))
								}
							}
						}
					case <-f.quit:
						return
					}
				}
			}

		}(orderPair, strategy)
	}

	return nil
}

func (f *filler) login() error {
	jwt, err := f.restClient.Login()
	if err != nil {
		return err
	}

	return f.restClient.SetJwt(jwt)
}

func (f *filler) fill(strategy Strategy, order model.Order) (bool, error) {
	if strategy.Match(order) {
		from, to, fromAsset, toAsset, err := model.ParseOrderPair(order.OrderPair)
		if err != nil {
			return false, err
		}

		// When we fill the order, send address is of the `to` chain, receive address is of the `from` chain. And we
		// assume one of the chain will be native bitcoin chain.
		sendAddr := f.ethWallets[to].Address().Hex()
		receiveAddr := f.btcWallet.Address().EncodeAddress()
		if from.IsBTC() {
			sendAddr = f.btcWallet.Address().EncodeAddress()
			receiveAddr = f.ethWallets[to].Address().Hex()
		}

		// Check if we have enough eth to cover the gas
		ethChain := to
		if from.IsEVM() {
			ethChain = from
		}
		wallet := f.ethWallets[ethChain]
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ethBalance, err := wallet.Balance(ctx, nil, true)
		if err != nil {
			return false, err
		}
		if ethBalance.Cmp(big.NewInt(1e16)) <= 0 {
			return false, fmt.Errorf("%v balance too low", ethChain)
		}

		// Check if we have enough tokens to execute the order
		chain := to
		asset := fromAsset
		if to.IsBTC() {
			chain, asset = from, toAsset
		}
		vb, err := f.virtualBalance(chain, asset)
		if err != nil {
			return false, err
		}
		orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
		if !ok {
			return false, fmt.Errorf("fail to get order amount")
		}
		if vb.Cmp(orderAmount) < 0 {
			return false, fmt.Errorf("insufficient balance")
		}

		if err := f.restClient.FillOrder(order.ID, sendAddr, receiveAddr); err != nil {
			return false, err
		}

	}
	return false, nil
}

func (f *filler) virtualBalance(chain model.Chain, asset model.Asset) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Find pending orders we haven't initiated
	var signer string
	for _, wallet := range f.ethWallets {
		signer = wallet.Address().Hex()
		break
	}
	orders, err := f.restClient.GetOrders(rest.GetOrdersFilter{
		Taker:   signer,
		Status:  int(model.Filled),
		Verbose: true,
	})
	if err != nil {
		return nil, err
	}
	pendingAmount := big.NewInt(0)
	for _, order := range orders {
		if order.FollowerAtomicSwap.Chain == chain && order.FollowerAtomicSwap.Asset == asset && order.FollowerAtomicSwap.Status < model.Initiated {
			orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
			if !ok {
				return nil, err
			}
			pendingAmount.Add(pendingAmount, orderAmount)
		}
	}

	if chain.IsBTC() {
		balance, err := f.btcWallet.Balance(ctx)
		if err != nil {
			return nil, err
		}
		return big.NewInt(balance - pendingAmount.Int64()), nil
	} else {
		tokenAddr := common.HexToAddress(asset.SecondaryID())
		balance, err := f.ethWallets[chain].Balance(ctx, &tokenAddr, true)
		if err != nil {
			return nil, err
		}
		return balance.Sub(balance, pendingAmount), nil
	}
}
