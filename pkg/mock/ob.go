package mock

import (
	"github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
)

type OrderbookClient struct {
	FuncFillOrder func(uint, string, string) error
	FuncLogin     func() (string, error)
}

func NewOrderbookClient() OrderbookClient {
	return OrderbookClient{}
}

func (o OrderbookClient) FillOrder(orderID uint, sendAddress, receiveAddress string) error {
	if o.FuncFillOrder != nil {
		return o.FuncFillOrder(orderID, sendAddress, receiveAddress)
	}
	return nil
}

func (o OrderbookClient) CreateOrder(sendAddress, receiveAddress, orderPair, sendAmount, receiveAmount, secretHash string) (uint, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) GetOrder(id uint) (model.Order, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) GetOrders(filter rest.GetOrdersFilter) ([]model.Order, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) GetFollowerInitiateOrders() ([]model.Order, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) GetFollowerRedeemOrders() ([]model.Order, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) GetInitiatorInitiateOrders() ([]model.Order, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) GetFollowerRefundedOrders() ([]model.Order, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) FollowerWaitForRedeemOrders() ([]model.Order, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) InitiatorWaitForInitiateOrders() ([]model.Order, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) GetInitiatorRedeemOrders() ([]model.Order, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) GetLockedValue(user string, chain string) (int64, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) SetJwt(token string) error {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) Health() (string, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) GetNonce() (string, error) {
	// TODO implement me
	panic("implement me")
}

func (o OrderbookClient) Login() (string, error) {
	if o.FuncLogin != nil {
		return o.FuncLogin()
	}
	return testutil.RandomString(), nil
}

type OrderbookWsClient struct {
	FuncListen    func() <-chan interface{}
	FuncSubscribe func(string)
}

func NewOrderbookWsClient() OrderbookWsClient {
	return OrderbookWsClient{}
}

func (o OrderbookWsClient) Listen() <-chan interface{} {
	if o.FuncListen != nil {
		return o.FuncListen()
	}
	return nil
}

func (o OrderbookWsClient) Subscribe(msg string) {
	if o.FuncSubscribe != nil {
		o.FuncSubscribe(msg)
	}
	return
}
