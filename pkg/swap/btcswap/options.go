package btcswap

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/waddrmgr"
)

const DefaultMinRelayFee = 1

type Options struct {
	Network     *chaincfg.Params
	AddressType waddrmgr.AddressType
	FeeTier     string
	MinRelayFee int
}

func NewWalletOptions(network *chaincfg.Params) Options {
	switch network.Name {
	case chaincfg.MainNetParams.Name:
		return OptionsMainnet()
	case chaincfg.TestNet3Params.Name:
		return OptionsTestnet()
	case chaincfg.RegressionNetParams.Name:
		return OptionsRegression()
	default:
		panic(fmt.Sprintf("unknown network = %v", network.Name))
	}
}

func OptionsMainnet() Options {
	return Options{
		Network:     &chaincfg.MainNetParams,
		AddressType: waddrmgr.WitnessPubKey,
		FeeTier:     "high",
		MinRelayFee: DefaultMinRelayFee,
	}
}

func OptionsTestnet() Options {
	return Options{
		Network:     &chaincfg.TestNet3Params,
		AddressType: waddrmgr.WitnessPubKey,
		FeeTier:     "medium",
		MinRelayFee: DefaultMinRelayFee,
	}
}

func OptionsRegression() Options {
	return Options{
		Network:     &chaincfg.RegressionNetParams,
		AddressType: waddrmgr.WitnessPubKey,
		FeeTier:     "low",
		MinRelayFee: DefaultMinRelayFee,
	}
}

func (opts Options) WithNetwork(network *chaincfg.Params) Options {
	opts.Network = network
	return opts
}

func (opts Options) WithFeeTier(feeTier string) Options {
	opts.FeeTier = feeTier
	return opts
}

func (opts Options) WithAddressType(addressType waddrmgr.AddressType) Options {
	opts.AddressType = addressType
	return opts
}

func (opts Options) WithMinRelayFee(min int) Options {
	opts.MinRelayFee = min
	return opts
}
