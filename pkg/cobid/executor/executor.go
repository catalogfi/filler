package executor

import (
	"github.com/catalogfi/cobi/pkg/swap"
	"github.com/catalogfi/ob/model"
)

type Executor interface {
	// Chain() model.Chain

	Start()

	Stop()
}

type Executors []Executor

func (exes Executors) Start() {
	for _, exe := range exes {
		exe.Start()
	}
}

func (exes Executors) Stop() {
	for _, exe := range exes {
		exe.Stop()
	}
}

type ActionItem struct {
	Action swap.Action
	Swap   *model.AtomicSwap
}
