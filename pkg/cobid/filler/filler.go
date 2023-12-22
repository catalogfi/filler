package filler

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	obStore "github.com/catalogfi/orderbook/store"
	"go.uber.org/zap"
)

type Filler interface {
	Start() error
	Stop()
}

type filler struct {
	btcAddress string
	ethAddress string
	restClient rest.Client
	wSclient   rest.WSClient
	strategy   Strategy
	store      store.Store
	logger     *zap.Logger
	quit       chan struct{}
	execWg     *sync.WaitGroup
}

func NewFiller(
	btcAddress string,
	ethAddress string,
	restClient rest.Client,
	wSclient rest.WSClient,
	strategy Strategy,
	store store.Store,
	logger *zap.Logger,

) (Filler, error) {
	toChain, fromChain, _, _, err := model.ParseOrderPair(strategy.orderPair)
	if err != nil {
		return nil, err
	}

	if fromChain.IsBTC() {
		if err := obStore.CheckAddress(fromChain, btcAddress); err != nil {
			return nil, err
		}
		if err := obStore.CheckAddress(toChain, ethAddress); err != nil {
			return nil, err
		}
	} else {
		if err := obStore.CheckAddress(toChain, btcAddress); err != nil {
			return nil, err
		}
		if err := obStore.CheckAddress(fromChain, ethAddress); err != nil {
			return nil, err
		}
	}

	return &filler{
		btcAddress: btcAddress,
		ethAddress: ethAddress,
		restClient: restClient,
		wSclient:   wSclient,
		strategy:   strategy,
		store:      store,
		logger:     logger,
		quit:       make(chan struct{}),
		execWg:     new(sync.WaitGroup),
	}, nil
}

/*
- will gracefully stop all the fillers
*/
func (f *filler) Stop() {
	defer func() {
		close(f.quit)

	}()
	f.quit <- struct{}{}
	f.execWg.Wait()
}

func (f *filler) Start() error {
	defer f.execWg.Done()
	// to enable blocking stop message
	f.execWg.Add(1)

	// ctx, cancel := context.WithCancel(context.Background())
	expSetBack := time.Second

	_, fromChain, _, _, err := model.ParseOrderPair(f.strategy.orderPair)
	if err != nil {
		return err
	}

	fromAddress := f.ethAddress
	toAddress := f.btcAddress

	if fromChain.IsBTC() {
		fromAddress, toAddress = toAddress, fromAddress
	}

CONNECTIONLOOP:
	for {

		// If JWT expires, login again
		jwt, err := f.restClient.Login()
		if err != nil {
			f.logger.Error("failed logging in", zap.Error(err))

			time.Sleep(expSetBack) // Wait for expSetBack before retrying
			continue
		}

		if err := f.restClient.SetJwt(jwt); err != nil {
			f.logger.Error("failed setting jwt", zap.Error(err))
			continue
		}

		// connect to the websocket and subscribe on the signer's address also subscribe based on strategy
		f.logger.Info("subscribing to socket")
		// connect to the websocket and subscribe on the signer's address
		f.wSclient.Subscribe(fmt.Sprintf("subscribe::%v", f.strategy.orderPair))
		respChan := f.wSclient.Listen()
	SIGNALOOP:
		for {

			select {
			case resp, ok := <-respChan:
				if !ok {
					break SIGNALOOP
				}
				expSetBack = time.Second
				switch response := resp.(type) {
				case rest.WebsocketError:
					break SIGNALOOP
				case rest.OpenOrders:
					// fill orders
					orders := response.Orders
					f.logger.Info("received orders from the order book", zap.Int("count", len(orders)))
					for _, order := range orders {
						if order.Price < f.strategy.price {
							f.logger.Info("order price is less than the strategy price", zap.Float64("order price", order.Price), zap.Float64("strategy price", f.strategy.price))
							continue
						}

						if len(f.strategy.makers) > 0 && !contains(f.strategy.makers, order.Maker) {
							f.logger.Info("maker is not in the list of makers", zap.String("maker", order.Maker))
							continue
						}

						orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
						if !ok {
							f.logger.Error("failed to parse order amount")
							continue
						}

						if (f.strategy.minAmount.Cmp(big.NewInt(0)) != 0 && orderAmount.Cmp(f.strategy.minAmount) < 0) ||
							(f.strategy.maxAmount.Cmp(big.NewInt(0)) != 0 && orderAmount.Cmp(f.strategy.maxAmount) > 0) {
							f.logger.Info("order amount is out of range", zap.String("order amount", orderAmount.String()), zap.String("min amount", f.strategy.minAmount.String()), zap.String("max amount", f.strategy.maxAmount.String()))
							continue
						}

						if err := f.restClient.FillOrder(order.ID, fromAddress, toAddress); err != nil {
							f.logger.Error("failed to fill the order ❌", zap.Uint("id", order.ID), zap.Error(err))
							continue
						}

						if err = f.store.PutSecret(order.SecretHash, nil, uint64(order.ID)); err != nil {
							f.logger.Error("failed storing secret hash: %v", zap.Error(err))
							continue
						}
						f.logger.Info("filled order ✅", zap.Uint("id", order.ID))

					}
				}
			case <-f.quit:
				f.logger.Info("received quit channel signal")
				break CONNECTIONLOOP
			}

		}
		time.Sleep(expSetBack)
		if expSetBack < (8 * time.Second) {
			expSetBack *= 2
		}
	}
	return nil
}

func contains(slice []string, item string) bool {
	for _, a := range slice {
		if a == item {
			return true
		}
	}
	return false
}
