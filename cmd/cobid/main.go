package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/bwmarrin/discordgo"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/pkg/cobid"
	"github.com/catalogfi/cobi/pkg/cobid/filler"
	"github.com/catalogfi/cobi/pkg/util"
	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var DiscordWebhookRegex = `^https://discord.com/api/webhooks/(?P<wid>\d+)/(?P<token>.+)$`

func main() {
	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.EncoderConfig.TimeKey = ""
	loggerConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	logger, err := loggerConfig.Build()
	if err != nil {
		panic(err)
	}

	// Check if we have discord webhook setup
	webhookURL := os.Getenv("DISCORD_WEBHOOK")
	if webhookURL != "" {
		// Validate the webhook url
		reg := regexp.MustCompile(DiscordWebhookRegex)
		if !reg.MatchString(webhookURL) {
			panic("invalid discord webhook url")
		}
		matches := reg.FindStringSubmatch(webhookURL)
		if len(matches) != 3 {
			panic("invalid matches")
		}

		// Parse the webhook id and token, adding a discord hook to our logger
		id, token := matches[1], matches[2]
		disClient, err := discordgo.New("")
		if err != nil {
			panic(err)
		}
		logger = logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
			if entry.Level == zap.ErrorLevel {
				// Ignore the wb reconnection error for now
				if strings.Contains(entry.Message, "failed to read message from") {
					return nil
				}
				_, err := disClient.WebhookExecute(id, token, true, &discordgo.WebhookParams{
					Content: entry.Message,
				})
				return err
			}
			return nil
		}))
	}

	// Decode key
	keyStr := parseRequiredEnv("PRIVATE_KEY")
	keyBytes, err := hex.DecodeString(keyStr)
	if err != nil {
		panic(err)
	}
	key, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		panic(err)
	}

	// Parse Chain configs
	btcConfig, evmConfigs, err := ParseChainConfig()
	if err != nil {
		panic(err)
	}

	// Get addresses for filler strategy
	ethAddr := crypto.PubkeyToAddress(key.PublicKey)
	keyBytesHash := btcutil.Hash160(util.EcdsaToBtcec(key).PubKey().SerializeCompressed())
	btcAddr, err := btcutil.NewAddressWitnessPubKeyHash(keyBytesHash, btcConfig.Chain.Params())
	if err != nil {
		panic(err)
	}

	// Generate filler strategy
	var strategies filler.Strategies
	switch btcConfig.Chain.Params().Name {
	case chaincfg.MainNetParams.Name:
		strategies = MainnetStrategies(ethAddr, btcAddr)
	case chaincfg.TestNet3Params.Name:
		strategies = TestnetStrategies(ethAddr, btcAddr)
	default:
		panic(fmt.Sprintf("unknown network = %v", btcConfig.Chain.Params().Name))
	}

	// Init and start cobid
	config := cobid.Config{
		Key:          parseRequiredEnv("PRIVATE_KEY"),
		OrderbookURL: parseRequiredEnv("ORDERBOOK_URL"),
		RedisURL:     parseRequiredEnv("REDISCLOUD_URL"),
		Btc:          btcConfig,
		Evms:         evmConfigs,
		Strategies:   strategies,
	}
	estimator := InitFeeEstimator(btcConfig.Chain.Params())
	cobi, err := cobid.NewCobi(config, logger, estimator)
	if err != nil {
		panic(err)
	}
	if err := cobi.Start(); err != nil {
		panic(err)
	}
	defer cobi.Stop()

	// waiting system signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGKILL)
	<-sigs
}

func ParseNetwork() (model.Chain, model.Chain, error) {
	network := os.Getenv("NETWORK")
	switch network {
	case "mainnet":
		return model.Bitcoin, model.Ethereum, nil
	case "testnet":
		return model.BitcoinTestnet, model.EthereumSepolia, nil
	default:
		return "", "", fmt.Errorf("unknown network = %v", network)
	}
}

func parseRequiredEnv(name string) string {
	val := os.Getenv(name)
	if val == "" {
		panic(fmt.Sprintf("env '%v' not set", name))
	}
	return val
}

func TestnetStrategies(ethAddr common.Address, btcAddr btcutil.Address) []filler.Strategy {
	log.Print("btcAddress = ", btcAddr.EncodeAddress())
	log.Print("ethAddress = ", ethAddr.Hex())

	return []filler.Strategy{
		{
			OrderPair: "bitcoin_testnet-ethereum_sepolia:0x9ceD08aeE17Fbc333BB7741Ec5eB2907b0CA4241",
			Makers:    nil,
			MinAmount: big.NewInt(1000),
			MaxAmount: big.NewInt(100000),
			Fee:       10,
		},
		{
			OrderPair: "ethereum_sepolia:0x9ceD08aeE17Fbc333BB7741Ec5eB2907b0CA4241-bitcoin_testnet",
			Makers:    nil,
			MinAmount: big.NewInt(1000),
			MaxAmount: big.NewInt(100000),
			Fee:       10,
		},
		{
			OrderPair: "bitcoin_testnet-ethereum_testpolygonzk:0xd0D4553DD6FD162B46423947BF3c0cf8d692Aa66",
			Makers:    nil,
			MinAmount: big.NewInt(1000),
			MaxAmount: big.NewInt(100000),
			Fee:       10,
		},
		{
			OrderPair: "ethereum_testpolygonzk:0xd0D4553DD6FD162B46423947BF3c0cf8d692Aa66-bitcoin_testnet",
			Makers:    nil,
			MinAmount: big.NewInt(1000),
			MaxAmount: big.NewInt(100000),
			Fee:       10,
		},
		{
			OrderPair: "ethereum_sepolia:0x9ceD08aeE17Fbc333BB7741Ec5eB2907b0CA4241-ethereum_testpolygonzk:0xd0D4553DD6FD162B46423947BF3c0cf8d692Aa66",
			Makers:    nil,
			MinAmount: big.NewInt(1000),
			MaxAmount: big.NewInt(1e8),
			Fee:       10,
		},
		{
			OrderPair: "ethereum_testpolygonzk:0xd0D4553DD6FD162B46423947BF3c0cf8d692Aa66-ethereum_sepolia:0x9ceD08aeE17Fbc333BB7741Ec5eB2907b0CA4241",
			Makers:    nil,
			MinAmount: big.NewInt(1000),
			MaxAmount: big.NewInt(1e8),
			Fee:       10,
		},
	}
}

func MainnetStrategies(ethAddr common.Address, btcAddr btcutil.Address) []filler.Strategy {
	log.Print("btcAddress = ", btcAddr.EncodeAddress())
	log.Print("ethAddress = ", ethAddr.Hex())
	return []filler.Strategy{
		{
			OrderPair: "bitcoin-ethereum:0xA5E38d098b54C00F10e32E51647086232a9A0afD",
			Makers:    nil,
			MinAmount: big.NewInt(100000),
			MaxAmount: big.NewInt(150000000),
			Fee:       10,
		},
		{
			OrderPair: "ethereum:0xA5E38d098b54C00F10e32E51647086232a9A0afD-bitcoin",
			Makers:    nil,
			MinAmount: big.NewInt(100000),
			MaxAmount: big.NewInt(150000000),
			Fee:       10,
		},
		{
			OrderPair: "bitcoin-ethereum_arbitrum:0x203DAC25763aE783Ad532A035FfF33d8df9437eE",
			Makers:    nil,
			MinAmount: big.NewInt(100000),
			MaxAmount: big.NewInt(150000000),
			Fee:       10,
		},
		{
			OrderPair: "ethereum_arbitrum:0x203DAC25763aE783Ad532A035FfF33d8df9437eE-bitcoin",
			Makers:    nil,
			MinAmount: big.NewInt(100000),
			MaxAmount: big.NewInt(150000000),
			Fee:       10,
		},
	}
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

func InitFeeEstimator(params *chaincfg.Params) btc.FeeEstimator {
	switch params.Name {
	case chaincfg.MainNetParams.Name:
		return btc.NewMempoolFeeEstimator(params, btc.MempoolFeeAPI, 15*time.Second)
	case chaincfg.TestNet3Params.Name:
		return btc.NewMempoolFeeEstimator(params, btc.MempoolFeeAPITestnet, 30*time.Second)
	case chaincfg.RegressionNetParams.Name:
		return btc.NewFixFeeEstimator(1)
	default:
		panic("unknown network")
	}
}
