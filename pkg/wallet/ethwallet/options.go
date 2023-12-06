package ethwallet

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Options struct {
	ChainID   *big.Int
	TokenAddr common.Address
	SwapAddr  common.Address
	BlockStep uint64
}

func OptionsMainnet(tokenAddr, swapAddr common.Address) Options {
	return Options{
		ChainID:   big.NewInt(1),
		TokenAddr: common.Address{},
		SwapAddr:  common.Address{},
		BlockStep: 1000,
	}
}

func (opts Options) WithChainID(id *big.Int) Options {
	opts.ChainID = id
	return opts
}

func (opts Options) WithTokenAddr(tokenAddr common.Address) Options {
	opts.TokenAddr = tokenAddr
	return opts
}

func (opts Options) WithSwapAddr(swapAddr common.Address) Options {
	opts.SwapAddr = swapAddr
	return opts
}

func (opts Options) WithBlockStep(step uint64) Options {
	opts.BlockStep = step
	return opts
}
