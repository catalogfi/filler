package executor

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/swap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"go.uber.org/zap"
)

type ActionItem struct {
	Action swap.Action
	Swap   *model.AtomicSwap
}

type Executor interface {

	// Chain indicates which chain the executor is operating on.
	Chain() model.Chain

	// Execute the atomic swap with the given action.
	Execute(action swap.Action, swap *model.AtomicSwap)
}

// Executors contains a collection of Executor and will distribute task to different executors accordingly.
type Executors interface {

	// Start listening orders and processing the swaps depending on status.
	Start()

	// Stop the Executors
	Stop()
}

type executors struct {
	logger  *zap.Logger
	exes    map[model.Chain]Executor
	address string
	client  rest.WSClient
	store   Store
	quit    chan struct{}
	wg      *sync.WaitGroup
}

func New(logger *zap.Logger, exes []Executor, address string, client rest.WSClient, store Store) Executors {
	exeMap := map[model.Chain]Executor{}
	for _, exe := range exes {
		exeMap[exe.Chain()] = exe
	}
	return executors{
		logger:  logger.With(zap.String("service", "executor")),
		exes:    exeMap,
		address: strings.ToLower(address),
		client:  client,
		store:   store,
		quit:    make(chan struct{}, 1),
		wg:      new(sync.WaitGroup),
	}
}

func (exe executors) Start() {
	exe.wg.Add(1)
	go func() {
		defer exe.wg.Done()

		for {
			exe.logger.Info(fmt.Sprintf("subscribing to orders of %v", exe.address))
			exe.client.Subscribe(fmt.Sprintf("subscribe::%v", exe.address))
			respChan := exe.client.Listen()

		InnerLoop:
			for {
				select {
				case resp, ok := <-respChan:
					if !ok {
						break InnerLoop
					}

					switch response := resp.(type) {
					case rest.WebsocketError:
						break InnerLoop
					case rest.UpdatedOrders:
						for _, order := range response.Orders {
							if err := exe.processOrder(order); err != nil {
								exe.logger.Error("process order", zap.Error(err))
							}
						}
					}
				case <-exe.quit:
					return
				}
			}

			time.Sleep(5 * time.Second)
		}
	}()
}

func (exe executors) Stop() {
	if exe.quit != nil {
		close(exe.quit)
		exe.wg.Wait()
		exe.quit = nil
	}
}

func (exe executors) processOrder(order model.Order) error {
	if order.Status == model.Filled {
		order.FollowerAtomicSwap.SecretHash = order.SecretHash  // this is not populated by the orderbook
		order.InitiatorAtomicSwap.SecretHash = order.SecretHash // this is not populated by the orderbook

		// We're the maker
		if order.Maker == exe.address {
			if order.InitiatorAtomicSwap.Status == model.NotStarted {
				// initiate the InitiatorAtomicSwap
				exe.execute(swap.ActionInitiate, order.InitiatorAtomicSwap)
			} else if order.InitiatorAtomicSwap.Status == model.Initiated &&
				order.FollowerAtomicSwap.Status == model.Initiated {
				// redeem the FollowerAtomicSwap
				secretHash, err := hex.DecodeString(order.FollowerAtomicSwap.SecretHash)
				if err != nil {
					return err
				}
				secret, err := exe.store.Secret(secretHash)
				if err != nil {
					return err
				}
				order.FollowerAtomicSwap.Secret = hex.EncodeToString(secret)
				exe.execute(swap.ActionRedeem, order.FollowerAtomicSwap)
			} else if order.InitiatorAtomicSwap.Status == model.Expired {
				// refund the InitiatorAtomicSwap
				exe.execute(swap.ActionRefund, order.InitiatorAtomicSwap)
			}
		}

		// We're the taker
		if order.Taker == exe.address {
			if order.InitiatorAtomicSwap.Status == model.Initiated &&
				order.FollowerAtomicSwap.Status == model.NotStarted {
				// initiate the FollowerAtomicSwap
				exe.execute(swap.ActionInitiate, order.FollowerAtomicSwap)
			} else if order.InitiatorAtomicSwap.Status == model.Initiated &&
				order.FollowerAtomicSwap.Status == model.Redeemed {
				// redeem the InitiatorAtomicSwap
				if order.FollowerAtomicSwap.Secret == "" {
					return fmt.Errorf("missing secret")
				}
				order.InitiatorAtomicSwap.Secret = order.FollowerAtomicSwap.Secret
				exe.execute(swap.ActionRedeem, order.InitiatorAtomicSwap)
			} else if order.FollowerAtomicSwap.Status == model.Expired {
				// refund the FollowerAtomicSwap
				exe.execute(swap.ActionRefund, order.FollowerAtomicSwap)
			}
		}
	}
	return nil
}

func (exe executors) execute(action swap.Action, swap *model.AtomicSwap) {
	etr, ok := exe.exes[swap.Chain]
	if !ok {
		// Skip execution since the chain is not supported
		return
	}

	etr.Execute(action, swap)
}
