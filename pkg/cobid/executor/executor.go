package executor

import (
	"fmt"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"go.uber.org/zap"
)

type Executor interface {
	Start(account uint32, isIw bool)
	Done()
}

type Config struct {
}

type executor struct {
	orderBook string
	store     store.Store
	logger    *zap.Logger
	quit      chan struct{}
	wg        *sync.WaitGroup
}

/*
- will gracefully stop all the executors
*/
func (e *executor) Done() {
	e.quit <- struct{}{}
}

/*
- signer is the ethereum public address used to authenticate with
the orderbook server
- btcWallets and ethwallets respectively should be generated using
only one private, that is used by signer to create or fill the orders
*/
func (e *executor) Start(btcWallets map[model.Chain]btcswap.Wallet, ethWallets map[model.Chain]ethswap.Wallet, signer string) {
	obLogger := e.logger.With(zap.String("client", "orderbook"))
	expSb := time.Second

	execChans, quitChans := e.startChainExecutors(btcWallets, ethWallets)

CONNECTIONLOOP:
	for {
		e.logger.Info("subcribing to socket")
		// connect to the websocket and subscribe on the signer's address
		client := rest.NewWSClient(fmt.Sprintf("wss://%s/", e.orderBook), obLogger)
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
					e.logger.Info("recieved orders from the order book", zap.Int("count", len(orders)))
					for _, order := range orders {
						if order.Status == model.Filled {
							if initiatorExecChan, ok := execChans[order.InitiatorAtomicSwap.Chain]; ok {
								initiatorExecChan <- SwapMsg{
									Orderid: uint64(order.ID),
									Swap:    *order.InitiatorAtomicSwap,
								}
							}
							if followerExecChan, ok := execChans[order.InitiatorAtomicSwap.Chain]; ok {
								followerExecChan <- SwapMsg{
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
				for _, quitChan := range quitChans {
					quitChan <- struct{}{}
				}
				e.wg.Wait()
				break CONNECTIONLOOP
			}

		}
		time.Sleep(expSb)
		if expSb < (8 * time.Second) {
			expSb *= 2
		}
	}
}

func (e *executor) startChainExecutors(btcWallets map[model.Chain]btcswap.Wallet, ethWallets map[model.Chain]ethswap.Wallet) (map[model.Chain]chan SwapMsg, []chan struct{}) {
	var quitChannels []chan struct{}
	execChannels := make(map[model.Chain]chan SwapMsg)

	for chain, wallet := range btcWallets {
		quitChan := make(chan struct{})

		btcExec := NewBtcExecutor(wallet, e.store, e.logger.With(zap.String("chain", string(chain))), quitChan, e.wg)
		e.wg.Add(1)
		execChan := btcExec.Start()

		quitChannels = append(quitChannels, quitChan)
		execChannels[chain] = execChan

	}
	for chain, wallet := range ethWallets {
		quitChan := make(chan struct{})

		ethExec := NewEthExecutor(wallet, e.store, e.logger.With(zap.String("chain", string(chain))), quitChan, e.wg)
		e.wg.Add(1)
		execChan := ethExec.Start()

		quitChannels = append(quitChannels, quitChan)
		execChannels[chain] = execChan
	}
	return execChannels, quitChannels
}
