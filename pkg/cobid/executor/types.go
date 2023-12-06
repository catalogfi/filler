package executor

import "github.com/catalogfi/orderbook/model"

type SwapMsg struct {
	Orderid uint64
	Swap    model.AtomicSwap
}
