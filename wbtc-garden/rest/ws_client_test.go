package rest_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/catalogfi/cobi/wbtc-garden/mocks"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	mockCtrl      *gomock.Controller
	mockStore     *mocks.MockServerStore
	mockScreener  *mocks.MockScreener
	mockSecret    = "MOCK SECRET"
	errMock       = errors.New("mock error")
	mockOrderPair = "bitcoin-ethereum"
	mockAddress   = "0x1234567890123456789012345678901234567890"

	config = model.Config{}

	ctx    context.Context
	cancel context.CancelFunc
	client rest.WSClient
)

var _ = BeforeEach(func() {
	mockCtrl = gomock.NewController(GinkgoT())
	mockStore = mocks.NewMockServerStore(mockCtrl)
	mockScreener = mocks.NewMockScreener(mockCtrl)
	ctx, cancel = context.WithCancel(context.Background())
	go rest.NewServer(mockStore, config, zap.NewNop(), mockSecret, mockScreener).Run(ctx, ":8080")
	client = rest.NewWSClient("ws://localhost:8080", zap.NewNop())
})

var _ = AfterEach(func() {
	cancel()
	mockCtrl.Finish()
})

var _ = Describe("subscribe to order status updates", func() {
	It("should return the order if the status is already executed", func() {
		mockStore.EXPECT().GetOrder(uint(5)).Return(&model.Order{
			Status:              model.Executed,
			InitiatorAtomicSwap: &model.AtomicSwap{},
			FollowerAtomicSwap:  &model.AtomicSwap{},
		}, nil).Times(1)
		client.Subscribe("subscribe_5")
		listener := client.Listen()
		order := <-listener
		Expect(order.(rest.UpdatedOrder).Order.Status).To(Equal(model.Executed))
	})

	It("should return an updated order error if get order fails", func() {
		mockStore.EXPECT().GetOrder(uint(5)).Return(nil, errMock).Times(1)
		client.Subscribe("subscribe_5")
		listener := client.Listen()
		order := <-listener
		Expect(order.(rest.UpdatedOrder).Error).ToNot(BeEmpty())
	})

	It("should return a websocket error if the order id is not a uint64 value", func() {
		client.Subscribe("subscribe_5123132312739812738192381023920183199392101827918321676876876876876786876876768767687812738192381023920183199392101827918321676876876876876786876876768767687")
		listener := client.Listen()
		err := <-listener
		Expect(err.(rest.WebsocketError).Error).ToNot(BeEmpty())
	})

	It("should return order and error if the getorder returns an order and an error", func() {
		// Expect the function to be called 3 times
		callCount := 0
		mockStore.EXPECT().GetOrder(uint(5)).DoAndReturn(func(id uint) (*model.Order, error) {
			callCount++
			switch callCount {
			case 1:
				return &model.Order{
					Status:              model.Created,
					InitiatorAtomicSwap: &model.AtomicSwap{},
					FollowerAtomicSwap:  &model.AtomicSwap{},
				}, nil
			default:
				return nil, errMock
			}
		}).Times(2)

		client.Subscribe("subscribe_5")
		listener := client.Listen()
		order1 := <-listener
		Expect(order1.(rest.UpdatedOrder).Order.Status).To(Equal(model.Created))
		order2 := <-listener
		Expect(order2.(rest.UpdatedOrder).Error).ToNot(BeEmpty())
	})

	It("should send us order updates as they happen in the database", func() {
		// Expect the function to be called 4 times
		callCount := 0
		mockStore.EXPECT().GetOrder(uint(5)).DoAndReturn(func(id uint) (*model.Order, error) {
			callCount++
			switch callCount {
			case 1:
				return &model.Order{
					Status:              model.Unknown,
					InitiatorAtomicSwap: &model.AtomicSwap{},
					FollowerAtomicSwap:  &model.AtomicSwap{},
				}, nil
			case 2, 3:
				return &model.Order{
					Status:              model.Filled,
					InitiatorAtomicSwap: &model.AtomicSwap{},
					FollowerAtomicSwap:  &model.AtomicSwap{},
				}, nil
			default:
				return &model.Order{
					Status:              model.Executed,
					InitiatorAtomicSwap: &model.AtomicSwap{},
					FollowerAtomicSwap:  &model.AtomicSwap{},
				}, nil
			}
		}).Times(4)

		client.Subscribe("subscribe_5")
		listener := client.Listen()
		order1 := <-listener
		Expect(order1.(rest.UpdatedOrder).Order.Status).To(Equal(model.Unknown))
		order2 := <-listener
		Expect(order2.(rest.UpdatedOrder).Order.Status).To(Equal(model.Filled))
		order3 := <-listener
		Expect(order3.(rest.UpdatedOrder).Order.Status).To(Equal(model.Executed))
	})
})

var _ = Describe("subscribe to all open orders", func() {
	It("should return the order if the status is already executed", func() {
		mockStore.EXPECT().FilterOrders(
			"", "", "", mockOrderPair, "", model.Created, 0.0, 0.0, 0.0, 0.0, 0, 0, true,
		).Return(nil, errMock).Times(1)

		client.Subscribe(fmt.Sprintf("subscribe_%s", mockOrderPair))
		listener := client.Listen()
		order := <-listener
		Expect(order.(rest.OpenOrder).Error).ToNot(BeEmpty())
	})

	It("should return order and error if the getorder returns an order and an error", func() {
		callCount := 0
		mockStore.EXPECT().FilterOrders(
			"", "", "", mockOrderPair, "", model.Created, 0.0, 0.0, 0.0, 0.0, 0, 0, true,
		).DoAndReturn(func(maker, taker, orderPair, secretHash, sort string, status model.Status, minPrice, maxPrice float64, minAmount, maxAmount float64, page, perPage int, verbose bool) ([]model.Order, error) {
			callCount++
			switch callCount {
			case 1, 2:
				return []model.Order{{
					Model:               gorm.Model{ID: 1},
					Status:              model.Created,
					InitiatorAtomicSwap: &model.AtomicSwap{},
					FollowerAtomicSwap:  &model.AtomicSwap{},
				}}, nil
			default:
				return nil, errMock
			}
		}).Times(3)

		client.Subscribe(fmt.Sprintf("subscribe_%s", mockOrderPair))
		listener := client.Listen()
		order := <-listener
		Expect(order.(rest.OpenOrder).Order.Status).To(Equal(model.Created))
		order2 := <-listener
		Expect(order2.(rest.OpenOrder).Error).ToNot(BeEmpty())
	})
})

var _ = Describe("subscribe to orders on an address", func() {
	It("should return the error if it get orders fail", func() {
		mockStore.EXPECT().GetOrdersByAddress(mockAddress).Return(nil, errMock).Times(1)

		client.Subscribe(fmt.Sprintf("subscribe_%s", mockAddress))
		listener := client.Listen()
		order := <-listener
		Expect(order.(rest.UpdatedOrders).Error).ToNot(BeEmpty())
	})

	It("should return order and error if the getorder returns an order and an error", func() {
		callCount := 0
		mockStore.EXPECT().GetOrdersByAddress(mockAddress).DoAndReturn(func(address string) ([]model.Order, error) {
			callCount++
			switch callCount {
			case 1, 2:
				return []model.Order{{
					Model:               gorm.Model{ID: 1},
					Status:              model.Created,
					InitiatorAtomicSwap: &model.AtomicSwap{},
					FollowerAtomicSwap:  &model.AtomicSwap{},
				}}, nil
			case 3:
				return []model.Order{{
					Model:               gorm.Model{ID: 1},
					Status:              model.Filled,
					InitiatorAtomicSwap: &model.AtomicSwap{},
					FollowerAtomicSwap:  &model.AtomicSwap{},
				}}, nil
			default:
				return nil, errMock
			}
		}).Times(4)

		client.Subscribe(fmt.Sprintf("subscribe_%s", mockAddress))
		listener := client.Listen()
		order := <-listener
		Expect(order.(rest.UpdatedOrders).Error).To(BeEmpty())
		Expect(len(order.(rest.UpdatedOrders).Orders)).To(Equal(1))
		Expect(order.(rest.UpdatedOrders).Orders[0].Status).To(Equal(model.Created))
		order2 := <-listener
		Expect(len(order2.(rest.UpdatedOrders).Orders)).To(Equal(1))
		Expect(order2.(rest.UpdatedOrders).Orders[0].Status).To(Equal(model.Filled))
		order3 := <-listener
		Expect(order3.(rest.UpdatedOrders).Error).ToNot(BeEmpty())
	})
})

var _ = Describe("subscribe to orders on an address", func() {
	It("should return a websocket error if an invalid subscribe string is sent", func() {
		client.Subscribe("subscribe_<hello>")
		listener := client.Listen()
		order := <-listener
		Expect(order.(rest.WebsocketError).Error).ToNot(BeEmpty())
	})

	It("should return a websocket error if an invalid subscribe string is sent", func() {
		client.Subscribe("subscribe")
		listener := client.Listen()
		order := <-listener
		Expect(order.(rest.WebsocketError).Error).ToNot(BeEmpty())
	})
})
