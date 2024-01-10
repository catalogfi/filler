package ethswap

import (
	"fmt"
	"math/big"

	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/common"
)

type Options struct {
	ChainID  *big.Int
	SwapAddr common.Address
}

func NewOptions(chain model.Chain, swapAddr common.Address) Options {
	if !chain.IsEVM() {
		panic("not a evm chain")
	}
	var chainID *big.Int
	switch chain {
	case model.Ethereum:
		chainID = big.NewInt(1)
	case model.EthereumSepolia:
		chainID = big.NewInt(11155111)
	case model.EthereumLocalnet:
		chainID = big.NewInt(1337)
	case model.EthereumArbitrum:
		chainID = big.NewInt(1337)
	default:
		panic(fmt.Sprintf("unknown evm chain = %v", chain))
	}

	return Options{
		ChainID:  chainID,
		SwapAddr: swapAddr,
	}
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
