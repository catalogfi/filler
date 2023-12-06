package btcwallet

import "github.com/btcsuite/btcd/chaincfg"

type Options struct {
	Network         *chaincfg.Params
	FeeTier         string
	MinConfirmation uint64
}

func OptionsMainnet() Options {
	return Options{
		Network:         &chaincfg.MainNetParams,
		FeeTier:         "high",
		MinConfirmation: 6,
	}
}

func OptionsTestnet() Options {
	return Options{
		Network:         &chaincfg.TestNet3Params,
		FeeTier:         "medium",
		MinConfirmation: 1,
	}
}

func OptionsRegression() Options {
	return Options{
		Network:         &chaincfg.RegressionNetParams,
		FeeTier:         "low",
		MinConfirmation: 0,
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

func (opts Options) WithMinConf(minConf uint64) Options {
	opts.MinConfirmation = minConf
	return opts
}
