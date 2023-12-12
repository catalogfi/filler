package executor

import (
	"fmt"

	"github.com/catalogfi/orderbook/model"
)

type Options struct {
	Orderbook string
	BTCChain  model.Chain
	ETHChain  model.Chain
}

func TesnetOptions(orderBook string) Options {
	return Options{
		Orderbook: orderBook,
		BTCChain:  model.BitcoinTestnet,
		ETHChain:  model.EthereumSepolia,
	}
}
func RegtestOptions(orderBook string) Options {
	return Options{
		Orderbook: orderBook,
		BTCChain:  model.BitcoinRegtest,
		ETHChain:  model.EthereumLocalnet,
	}
}

func OptionsWithChains(orderBook string, btcChain model.Chain, ethChain model.Chain) (Options, error) {
	if !btcChain.IsBTC() {
		return Options{}, fmt.Errorf("not a valid bitcoin chain")
	}
	if !btcChain.IsEVM() {
		return Options{}, fmt.Errorf("not a valid ethereum chain")
	}
	return Options{
		Orderbook: orderBook,
		BTCChain:  btcChain,
		ETHChain:  ethChain,
	}, nil
}
