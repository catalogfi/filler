package executor

import "github.com/catalogfi/orderbook/model"

type SwapMsg struct {
	OrderId uint64
	// CounterSwapStatus model.SwapStatus
	Type   ExecutorType
	Swap   model.AtomicSwap
	Action ExecuteAction
}

type ExecutorType int

const (
	Initiator ExecutorType = iota
	Follower
)

type ExecuteAction int

const (
	Initiate ExecuteAction = iota
	Redeem
	Refund
)
