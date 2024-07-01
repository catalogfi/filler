package ethswap

import (
	"fmt"
	"math/big"
	"time"

	"github.com/catalogfi/ob/model"
	"github.com/ethereum/go-ethereum/common"
)

type Options struct {
	ChainID  *big.Int
	SwapAddr common.Address
	Timeout  time.Duration
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
		chainID = big.NewInt(31337)
	case model.EthereumArbitrumLocalnet:
		chainID = big.NewInt(31338)
	case model.EthereumArbitrum:
		chainID = big.NewInt(42161)
	default:
		panic(fmt.Sprintf("unknown evm chain = %v", chain))
	}

	return Options{
		ChainID:  chainID,
		SwapAddr: swapAddr,
		Timeout:  5 * time.Second,
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

func (opts Options) WithTimeout(timeout time.Duration) Options {
	opts.Timeout = timeout
	return opts
}
