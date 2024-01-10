package filler

import (
	"fmt"
	"sync"

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
	strategies Strategies
	orderbook  string
	restClient rest.Client
	logger     *zap.Logger
	quit       chan struct{}
	wg         *sync.WaitGroup
}

func New(strategies Strategies, restClient rest.Client, orderbook string, logger *zap.Logger) Filler {
	return &filler{
		strategies: strategies,
		orderbook:  orderbook,
		restClient: restClient,
		logger:     logger,
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
				client := rest.NewWSClient(fmt.Sprintf("wss://%s/", f.orderbook), f.logger)
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
								// TODO : BALANCE CHECK TO MAKE SURE WE HAVE ENOUGH AMOUNT TO EXECUTE
								if strategy.Match(order) {
									if err := f.restClient.FillOrder(order.ID, strategy.SendAddress, strategy.ReceiveAddress); err != nil {
										f.logger.Error("❌ [FILL]", zap.Uint("id", order.ID), zap.Error(err))
										continue
									}

									f.logger.Info("✅ [FILL]", zap.Uint("id", order.ID))
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
