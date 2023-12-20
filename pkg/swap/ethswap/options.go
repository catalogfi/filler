package ethswap

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Options struct {
	ChainID  *big.Int
	SwapAddr common.Address
}

func OptionsMainnet(swapAddr common.Address) Options {
	return Options{
		ChainID:  big.NewInt(1),
		SwapAddr: swapAddr,
	}
}

func OptionsLocalnet(swapAddr common.Address) Options {
	return Options{
		ChainID:  big.NewInt(1337),
		SwapAddr: swapAddr,
	}
}

func (opts Options) WithChainID(id *big.Int) Options {
	opts.ChainID = id
	return opts
}

func (opts Options) WithSwapAddr(swapAddr common.Address) Options {
	opts.SwapAddr = swapAddr
	return opts
}
