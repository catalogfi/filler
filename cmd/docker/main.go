package main

import (
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/pkg/cobid"
	"github.com/catalogfi/cobi/pkg/cobid/creator"
	"github.com/catalogfi/cobi/pkg/cobid/filler"
	"github.com/catalogfi/orderbook/model"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.EncoderConfig.TimeKey = ""
	loggerConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	logger, err := loggerConfig.Build()
	if err != nil {
		panic(err)
	}

	btcConfig := cobid.BtcChainConfig{
		Chain:   model.BitcoinRegtest,
		Indexer: "http://host.docker.internal:3000",
	}
	evmConfigs := []cobid.EvmChainConfig{
		{
			Chain:       model.EthereumLocalnet,
			SwapAddress: "0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512",
			URL:         "http://host.docker.internal:8545",
		},
		{
			Chain:       model.EthereumArbitrumLocalnet,
			SwapAddress: "0xDc64a140Aa3E981100a9becA4E685f962f0cF6C9",
			URL:         "http://host.docker.internal:8546",
		},
	}

	fillerStrategies, creatorStrategies := LocalnetStratagies()

	// Init and start cobid
	config := cobid.Config{
		Key:               "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
		OrderbookURL:      "http://host.docker.internal:8080",
		OrderbookWSURL:    "ws://host.docker.internal:8080",
		RedisURL:          "http://host.docker.internal:6379",
		Btc:               btcConfig,
		Evms:              evmConfigs,
		FillerStrategies:  fillerStrategies,
		CreatorStrategies: creatorStrategies,
	}

	cobi, err := cobid.NewCobi(config, logger, btc.NewFixFeeEstimator(10))
	if err != nil {
		panic(err)
	}
	if err := cobi.Start(); err != nil {
		panic(err)
	}
	defer cobi.Stop()

	// waiting system signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
}

func ParseChainConfig() (cobid.BtcChainConfig, []cobid.EvmChainConfig, error) {
	// Parse network
	network := parseRequiredEnv("NETWORK")

	// Parse bitcoin
	btcConfig := cobid.BtcChainConfig{
		Chain:   model.BitcoinTestnet,
		Indexer: parseRequiredEnv("BITCOIN_INDEXER"),
	}
	if network == "mainnet" {
		btcConfig.Chain = model.Bitcoin
	}
	if network == "localnet" {
		btcConfig.Chain = model.BitcoinRegtest
	}

	// Parse evms config
	evms := parseRequiredEnv("EVMS")
	chains := []cobid.EvmChainConfig{}
	for _, chainStr := range strings.Split(evms, ",") {
		chain, err := model.ParseChain(chainStr)
		if err != nil {
			return cobid.BtcChainConfig{}, nil, err
		}
		if !chain.IsEVM() {
			return cobid.BtcChainConfig{}, nil, fmt.Errorf("invalid evm chain = %v", chain)
		}

		config := cobid.EvmChainConfig{
			Chain:       chain,
			SwapAddress: parseRequiredEnv(strings.ToUpper(string(chain)) + "_SWAP_CONTRACT"),
			URL:         parseRequiredEnv(strings.ToUpper(string(chain)) + "_URL"),
		}
		chains = append(chains, config)
	}
	return btcConfig, chains, nil
}

func parseRequiredEnv(name string) string {
	val := os.Getenv(name)
	if val == "" {
		panic(fmt.Sprintf("env '%v' not set", name))
	}
	return val
}

func LocalnetStratagies() ([]filler.Strategy, []creator.Strategy) {
	return []filler.Strategy{
			{
				OrderPair: "bitcoin_regtest-ethereum_localnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9",
				Makers:    nil,
				MinAmount: big.NewInt(1000),
				MaxAmount: big.NewInt(1e8),
				Fee:       10,
			},
			{
				OrderPair: "ethereum_localnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9-bitcoin_regtest",
				Makers:    nil,
				MinAmount: big.NewInt(1000),
				MaxAmount: big.NewInt(1e8),
				Fee:       10,
			},
			{
				OrderPair: "bitcoin_regtest-ethereum_arbitrumlocalnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9",
				Makers:    nil,
				MinAmount: big.NewInt(1000),
				MaxAmount: big.NewInt(1e8),
				Fee:       10,
			},
			{
				OrderPair: "ethereum_arbitrumlocalnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9-bitcoin_regtest",
				Makers:    nil,
				MinAmount: big.NewInt(1000),
				MaxAmount: big.NewInt(1e8),
				Fee:       10,
			},
			{
				OrderPair: "ethereum_localnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9-ethereum_arbitrumlocalnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9",
				Makers:    nil,
				MinAmount: big.NewInt(1000),
				MaxAmount: big.NewInt(1e8),
				Fee:       10,
			},
			{
				OrderPair: "ethereum_arbitrumlocalnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9-ethereum_localnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9",
				Makers:    nil,
				MinAmount: big.NewInt(1000),
				MaxAmount: big.NewInt(1e8),
				Fee:       10,
			},
		}, []creator.Strategy{
			{
				MinTimeInterval: 10,
				MaxTimeInterval: 60,
				Amount:          big.NewInt(25000),
				OrderPair:       "bitcoin_regtest-ethereum_localnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9",
				Fee:             10,
			},
			{
				MinTimeInterval: 10,
				MaxTimeInterval: 60,
				Amount:          big.NewInt(25000),
				OrderPair:       "ethereum_localnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9-bitcoin_regtest",
				Fee:             10,
			},
			{
				MinTimeInterval: 10,
				MaxTimeInterval: 60,
				Amount:          big.NewInt(25000),
				OrderPair:       "bitcoin_regtest-ethereum_arbitrumlocalnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9",
				Fee:             10,
			},
			{
				MinTimeInterval: 10,
				MaxTimeInterval: 60,
				Amount:          big.NewInt(25000),
				OrderPair:       "ethereum_arbitrumlocalnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9-bitcoin_regtest",
				Fee:             10,
			},
			{
				MinTimeInterval: 10,
				MaxTimeInterval: 60,
				Amount:          big.NewInt(25000),
				OrderPair:       "ethereum_localnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9-ethereum_arbitrumlocalnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9",
				Fee:             10,
			},
			{
				MinTimeInterval: 10,
				MaxTimeInterval: 60,
				Amount:          big.NewInt(25000),
				OrderPair:       "ethereum_arbitrumlocalnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9-ethereum_localnet:0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9",
				Fee:             10,
			},
		}
}
