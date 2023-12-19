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
	client    rest.WSClient
	options   Options
	store     store.Store
	logger    *zap.Logger
	quit      chan struct{}
	chainWg   *sync.WaitGroup
	execWg    *sync.WaitGroup
}

func NewExecutor(
	btcWallet btcswap.Wallet,
	ethWallet ethswap.Wallet,
	signer common.Address,
	client rest.WSClient,
	options Options,
	store store.Store,
	logger *zap.Logger,
) Executor {
	return &executor{
		btcWallet: btcWallet,
		ethWallet: ethWallet,
		signer:    signer,
		client:    client,
		store:     store,
		options:   options,
		logger:    logger,
		quit:      make(chan struct{}),
		chainWg:   new(sync.WaitGroup),
		execWg:    new(sync.WaitGroup),
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
	e.execWg.Wait()
}

/*
- signer is the ethereum public address used to authenticate with
the orderbook server
- btcWallets and ethwallets respectively should be generated using
only one private, that is used by signer to create or fill the orders
*/
func (e *executor) Start() {
	defer e.execWg.Done()
	// to enable blocking stop message
	e.execWg.Add(1)

	ctx, cancel := context.WithCancel(context.Background())
	expSetBack := time.Second

	// execChans, quitChans := e.startChainExecutors(btcWallets, ethWallets)
	ethExecChan := e.StartEthExecutor(ctx)
	e.chainWg.Add(1)

	btcExecChan := e.StartBtcExecutor(ctx)
	e.chainWg.Add(1)

CONNECTIONLOOP:
	for {
		e.logger.Info("subcribing to socket")
		// connect to the websocket and subscribe on the signer's address
		e.client.Subscribe(fmt.Sprintf("subscribe::%v", e.signer))
		respChan := e.client.Listen()
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
						var execType ExecutorType
						if order.Maker == e.signer.String() {
							execType = Initiator
						} else {
							execType = Follower
						}
						if order.Status == model.Filled {
							switch order.InitiatorAtomicSwap.Chain {
							case e.options.ETHChain:
								ethExecChan <- SwapMsg{
									OrderId:           uint64(order.ID),
									CounterSwapStatus: model.SwapStatus(order.FollowerAtomicSwap.Status),
									Type:              execType,
									Swap:              *order.InitiatorAtomicSwap,
								}
							case e.options.BTCChain:
								btcExecChan <- SwapMsg{
									OrderId:           uint64(order.ID),
									CounterSwapStatus: model.SwapStatus(order.FollowerAtomicSwap.Status),
									Type:              execType,
									Swap:              *order.InitiatorAtomicSwap,
								}
							}
							switch order.FollowerAtomicSwap.Chain {
							case e.options.ETHChain:
								ethExecChan <- SwapMsg{
									OrderId:           uint64(order.ID),
									CounterSwapStatus: model.SwapStatus(order.InitiatorAtomicSwap.Status),
									Type:              execType,
									Swap:              *order.FollowerAtomicSwap,
								}
							case e.options.BTCChain:
								btcExecChan <- SwapMsg{
									OrderId:           uint64(order.ID),
									CounterSwapStatus: model.SwapStatus(order.InitiatorAtomicSwap.Status),
									Type:              execType,
									Swap:              *order.FollowerAtomicSwap,
								}
							}
						}
					}
				}
				continue
			case <-e.quit:
				e.logger.Info("recieved quit channel signal")
				cancel()
				// waiting for executor to complete
				e.chainWg.Wait()
				break CONNECTIONLOOP
			}

		}
		time.Sleep(expSetBack)
		if expSetBack < (8 * time.Second) {
			expSetBack *= 2
		}
	}
}
