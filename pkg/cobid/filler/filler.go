package filler

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"go.uber.org/zap"
)

type Filler interface {
	// Start the filler to match orders, it's not blocking and will spawn a background goroutine.
	Start() error

	// Stop will gracefully shut down the Filler, it waits for all inner goroutines to finish.
	Stop()
}

type filler struct {
	logger     *zap.Logger
	strategies Strategies
	btcWallet  btcswap.Wallet
	ethWallets map[model.Chain]ethswap.Wallet
	dialer     func() rest.WSClient
	restClient rest.Client

	signer string
	quit   chan struct{}
	wg     *sync.WaitGroup
}

func New(strategies Strategies, btcWallet btcswap.Wallet, ethWallets map[model.Chain]ethswap.Wallet, restClient rest.Client, dialer func() rest.WSClient, logger *zap.Logger) Filler {
	var signer string
	for _, wallet := range ethWallets {
		signer = strings.ToLower(wallet.Address().Hex())
	}

	return &filler{
		logger:     logger,
		strategies: strategies,
		btcWallet:  btcWallet,
		ethWallets: ethWallets,
		dialer:     dialer,
		restClient: restClient,

		signer: signer,
		quit:   make(chan struct{}),
		wg:     new(sync.WaitGroup),
	}
}

func (f *filler) Start() error {
	for _, strategy := range f.strategies {
		unmatched := make(chan model.Order, 128)
		go f.match(strategy, unmatched)
		go f.fill(strategy.OrderPair, unmatched)
	}

	return nil
}

func (f *filler) Stop() {
	if f.quit != nil {
		close(f.quit)
		f.wg.Wait()
		f.quit = nil
	}
}

// match checks if the given order matches our strategy.
func (f *filler) match(strategy Strategy, ordersChan chan<- model.Order) {
	f.wg.Add(1)
	defer f.wg.Done()

	for {
		f.logger.Info("subscribing to orderPair", zap.String("orderPair", strategy.OrderPair))
		client := f.dialer()
		client.Subscribe(fmt.Sprintf("subscribe::%v", strategy.OrderPair))
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
						match, err := strategy.Match(order)
						if err != nil {
							f.logger.Debug("❌ [Not Match]", zap.Uint("id", order.ID), zap.Error(err))
						}
						if match {
							ordersChan <- order
							f.logger.Debug("✅ [Match]", zap.Uint("id", order.ID))
						}
					}
				}
			case <-f.quit:
				return
			}
		}
	}
}

func (f *filler) fill(orderPair string, ordersChan <-chan model.Order) {
	from, to, _, toAsset, err := model.ParseOrderPair(orderPair)
	if err != nil {
		f.logger.Panic("parse order pair", zap.Error(err))
	}

	// When we fill the order, send address is of the `to` chain, receive address is of the `from` chain. And we assume
	// one of the chain will be native bitcoin chain.
	sendAddr, receiveAddr, ethChain := "", "", to
	if from.IsBTC() {
		sendAddr = f.ethWallets[to].Address().Hex()
		receiveAddr = f.btcWallet.Address().EncodeAddress()
	} else {
		sendAddr = f.btcWallet.Address().EncodeAddress()
		receiveAddr = f.ethWallets[from].Address().Hex()
		ethChain = from
	}

	for order := range ordersChan {
		// Fill the order in the orderbook if we have enough funds to execute. If the funds is not enough, we wait and
		// check again later
		func() {
			interval := 15 * time.Second
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for ; ; <-ticker.C {
				if err := f.balanceCheck(to, ethChain, toAsset, order, interval); err != nil {
					f.logger.Error("balance not enough", zap.Error(err))
					continue
				}

				// Fill the order in the orderbook
				if err := f.restClient.FillOrder(order.ID, sendAddr, receiveAddr); err != nil {
					f.logger.Error("fill order", zap.Error(err))
					continue
				}
				f.logger.Info("✅ [Fill]", zap.Uint("id", order.ID))

				// Move on to next order
				return
			}
		}()
	}
}

func (f *filler) login() error {
	jwt, err := f.restClient.Login()
	if err != nil {
		return err
	}

	return f.restClient.SetJwt(jwt)
}

func (f *filler) balanceCheck(chain, ethChain model.Chain, asset model.Asset, order model.Order, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	amount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
	if !ok {
		return fmt.Errorf("failed to decode amount")
	}

	// Check we have enough ETH for gas ( >=0.1 )
	ethWallet := f.ethWallets[ethChain]
	ethBalance, err := ethWallet.Balance(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to get eth balance")
	}
	if ethBalance.Cmp(big.NewInt(1e17)) <= 0 {
		addr := ethWallet.Address()
		f.logger.Error("ETH balance low", zap.String("balance", ethBalance.String()), zap.String("addr", addr.Hex()))
		return fmt.Errorf("insufficent ETH")
	}

	if chain.IsBTC() {
		// Check if the balance is enough
		balance, err := f.btcWallet.Balance(ctx)
		if err != nil {
			return err
		}
		unexecuted, err := f.unexecutedAmount(chain, asset)
		if err != nil {
			return err
		}
		if balance < unexecuted.Int64()+amount.Int64() {
			return fmt.Errorf("balance is not enough, required = %v, has = %v", unexecuted.Int64()+amount.Int64(), balance)
		}
		return nil
	} else {
		wallet := f.ethWallets[chain]

		// Check if the balance is enough
		balance, err := wallet.TokenBalance(ctx, true)
		if err != nil {
			return err
		}
		unexecuted, err := f.unexecutedAmount(chain, asset)
		if err != nil {
			return err
		}
		required := unexecuted.Add(unexecuted, amount)
		if balance.Cmp(required) <= 0 {
			return fmt.Errorf("balance is not enough, required = %v, has = %v", required.String(), balance.String())
		}
		return nil
	}
}

func (f *filler) unexecutedAmount(chain model.Chain, asset model.Asset) (*big.Int, error) {
	filter := rest.GetOrdersFilter{
		Taker:   f.signer,
		Verbose: true,
		Status:  int(model.Filled),
	}
	orders, err := f.restClient.GetOrders(filter)
	if err != nil {
		return nil, err
	}
	amount := big.NewInt(0)
	for _, order := range orders {
		if order.FollowerAtomicSwap.Chain == chain &&
			order.FollowerAtomicSwap.Asset == asset &&
			order.FollowerAtomicSwap.Status == model.NotStarted {
			orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
			if !ok {
				return nil, err
			}
			amount.Add(amount, orderAmount)
		}
	}
	return amount, nil
}
