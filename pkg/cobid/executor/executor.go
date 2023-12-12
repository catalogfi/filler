package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

type Executor interface {
	Start()
	Stop()
}

type executor struct {
	btcWallet btcswap.Wallet
	ethWallet ethswap.Wallet
	signer    common.Address
	options   Options
	store     store.Store
	logger    *zap.Logger
	quit      chan struct{}
	wg        *sync.WaitGroup
}

func NewExecutor(
	btcWallet btcswap.Wallet,
	ethWallet ethswap.Wallet,
	signer common.Address,
	options Options,
	store store.Store,
	logger *zap.Logger,
	quit chan struct{},
) Executor {
	return &executor{
		btcWallet: btcWallet,
		ethWallet: ethWallet,
		signer:    signer,
		store:     store,
		options:   options,
		logger:    logger,
		quit:      quit,
		wg:        new(sync.WaitGroup),
	}

}

/*
- will gracefully stop all the executors
*/
func (e *executor) Stop() {
	defer func() {
		close(e.quit)

	}()
	e.quit <- struct{}{}
	e.wg.Done()
}

/*
- signer is the ethereum public address used to authenticate with
the orderbook server
- btcWallets and ethwallets respectively should be generated using
only one private, that is used by signer to create or fill the orders
*/
func (e *executor) Start() {
	// to enable blocking stop message
	e.wg.Add(1)

	ctx, cancel := context.WithCancel(context.Background())
	obLogger := e.logger.With(zap.String("client", "orderbook"))
	expSetBack := time.Second

	// execChans, quitChans := e.startChainExecutors(btcWallets, ethWallets)
	ethExecChan := e.StartEthExecutor(ctx)
	e.wg.Add(1)

	btcExecChan := e.StartBtcExecutor(ctx)
	e.wg.Add(1)

CONNECTIONLOOP:
	for {
		e.logger.Info("subcribing to socket")
		// connect to the websocket and subscribe on the signer's address
		client := rest.NewWSClient(fmt.Sprintf("wss://%s/", e.options.Orderbook), obLogger)
		client.Subscribe(fmt.Sprintf("subscribe::%v", e.signer))
		respChan := client.Listen()
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
				case rest.UpdatedOrders:
					// execute orders
					orders := response.Orders
					e.logger.Info("recieved orders from the order book", zap.Int("count", len(orders)))
					for _, order := range orders {
						if order.Status == model.Filled {
							switch order.InitiatorAtomicSwap.Chain {
							case e.options.ETHChain:
								ethExecChan <- SwapMsg{
									Orderid: uint64(order.ID),
									Swap:    *order.InitiatorAtomicSwap,
								}
							case e.options.BTCChain:
								btcExecChan <- SwapMsg{
									Orderid: uint64(order.ID),
									Swap:    *order.InitiatorAtomicSwap,
								}
							}
							switch order.FollowerAtomicSwap.Chain {
							case e.options.ETHChain:
								ethExecChan <- SwapMsg{
									Orderid: uint64(order.ID),
									Swap:    *order.FollowerAtomicSwap,
								}
							case e.options.BTCChain:
								btcExecChan <- SwapMsg{
									Orderid: uint64(order.ID),
									Swap:    *order.FollowerAtomicSwap,
								}
							}
						}
					}
				}
				continue
			case <-e.quit:
				e.logger.Info("recieved quit channel signal")
				cancel()
				// reducing 1 for itself
				e.wg.Done()
				// waiting for executor to complete
				e.wg.Wait()
				break CONNECTIONLOOP
			}

		}
		time.Sleep(expSetBack)
		if expSetBack < (8 * time.Second) {
			expSetBack *= 2
		}
	}
}
