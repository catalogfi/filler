package filler

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"go.uber.org/zap"
)

type Filler interface {
	Start()
	Stop()
}

// add strategies
type filler struct {
	btcWallet  btcswap.Wallet
	ethWallet  ethswap.Wallet
	restClient rest.Client
	wSclient   rest.WSClient
	strategy   Strategy
	store      store.Store
	logger     *zap.Logger
	quit       chan struct{}
	execWg     *sync.WaitGroup
}

func NewFiller(
	btcWallet btcswap.Wallet,
	ethWallet ethswap.Wallet,
	restClient rest.Client,
	wSclient rest.WSClient,
	strategy Strategy,
	store store.Store,
	logger *zap.Logger,
	quit chan struct{},
) Filler {
	return &filler{
		btcWallet:  btcWallet,
		ethWallet:  ethWallet,
		restClient: restClient,
		wSclient:   wSclient,
		strategy:   strategy,
		store:      store,
		logger:     logger,
		quit:       quit,
		execWg:     new(sync.WaitGroup),
	}
}

/*
- will gracefully stop all the fillers
*/
func (e *filler) Stop() {
	defer func() {
		close(e.quit)

	}()
	e.quit <- struct{}{}
	e.execWg.Wait()
}

/*
- signer is the ethereum public address used to authenticate with
the orderbook server
- btcWallets and ethwallets respectively should be generated using
only one private, that is used by signer to create or fill the orders
*/
func (e *filler) Start() {
	defer e.execWg.Done()
	// to enable blocking stop message
	e.execWg.Add(1)

	// ctx, cancel := context.WithCancel(context.Background())
	expSetBack := time.Second

CONNECTIONLOOP:
	for {

		_, fromChain, _, _, err := model.ParseOrderPair(e.strategy.orderPair)
		if err != nil {
			e.logger.Error("failed parsing order pair", zap.Error(err))
			return
		}

		fromAddress := e.ethWallet.Address().String()
		toAddress := e.btcWallet.Address().EncodeAddress()

		if fromChain.IsBTC() {
			fromAddress, toAddress = toAddress, fromAddress
		}

		// connect to the websocket and subscribe on the signer's address also subscribe based on strategy
		e.logger.Info("subcribing to socket")
		// connect to the websocket and subscribe on the signer's address
		e.wSclient.Subscribe(fmt.Sprintf("subscribe::%v", e.strategy.orderPair))
		respChan := e.wSclient.Listen()
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
					e.logger.Info("recieved orders from the order book", zap.Int("count", len(orders)))
					for _, order := range orders {
						fmt.Println("filling order", order.ID)
						//TODO virtual Balance check
						if order.Price < e.strategy.price {
							e.logger.Info("order price is less than the strategy price", zap.Float64("order price", order.Price), zap.Float64("strategy price", e.strategy.price))
							continue
						}

						if len(e.strategy.makers) > 0 && !contains(e.strategy.makers, order.Maker) {
							e.logger.Info("maker is not in the list of makers", zap.String("maker", order.Maker))
							continue
						}

						orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
						if !ok {
							e.logger.Error("failed to parse order amount")
							continue
						}

						if (e.strategy.minAmount.Cmp(big.NewInt(0)) != 0 && orderAmount.Cmp(e.strategy.minAmount) < 0) ||
							(e.strategy.maxAmount.Cmp(big.NewInt(0)) != 0 && orderAmount.Cmp(e.strategy.maxAmount) > 0) {
							e.logger.Info("order amount is out of range", zap.String("order amount", orderAmount.String()), zap.String("min amount", e.strategy.minAmount.String()), zap.String("max amount", e.strategy.maxAmount.String()))
							continue
						}

						if err := e.restClient.FillOrder(order.ID, fromAddress, toAddress); err != nil {
							e.logger.Error("failed to fill the order ❌", zap.Uint("id", order.ID), zap.Error(err))
							continue
						}

						if err = e.store.PutSecret(order.SecretHash, nil, uint64(order.ID)); err != nil {
							e.logger.Error("failed storing secret hash: %v", zap.Error(err))
							continue
						}
						e.logger.Info("filled order ✅", zap.Uint("id", order.ID))

					}
				}
				continue
			case <-e.quit:
				e.logger.Info("recieved quit channel signal")
				// cancel()
				// waiting for filler to complete
				// e.chainWg.Wait()
				break CONNECTIONLOOP
			}

		}
		time.Sleep(expSetBack)
		if expSetBack < (8 * time.Second) {
			expSetBack *= 2
		}
	}
}

func contains(slice []string, item string) bool {
	for _, a := range slice {
		if a == item {
			return true
		}
	}
	return false
}
