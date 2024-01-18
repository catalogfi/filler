package cobid

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/pkg/cobid/executor"
	"github.com/catalogfi/cobi/pkg/cobid/filler"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/cobi/pkg/util"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

type Cobid struct {
	// creator   creator.Creator
	executors executor.Executors
	filler    filler.Filler
}

type BtcChainConfig struct {
	Chain   model.Chain
	Indexer string
}

type EvmChainConfig struct {
	Chain       model.Chain
	SwapAddress string
	URL         string
}

type Config struct {
	Key          string
	OrderbookURL string
	RedisURL     string
	Btc          BtcChainConfig   // chain of the native bitcoin
	Evms         []EvmChainConfig // target evm chains for wbtc
	Strategies   []filler.Strategy
}

func NewCobi(config Config, estimator btc.FeeEstimator) (Cobid, error) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return Cobid{}, err
	}

	// Decode key
	keyBytes, err := hex.DecodeString(config.Key)
	if err != nil {
		return Cobid{}, err
	}
	key, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		return Cobid{}, err
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)

	// Filler
	client := rest.NewClient(fmt.Sprintf("https://%s", config.OrderbookURL), config.Key)
	token, err := client.Login()
	if err != nil {
		return Cobid{}, err
	}
	if err := client.SetJwt(token); err != nil {
		return Cobid{}, err
	}

	// Storage
	storage, err := executor.NewRedisStore(config.RedisURL)
	if err != nil {
		return Cobid{}, err
	}

	// Bitcoin wallet and executor
	indexer := btc.NewElectrsIndexerClient(logger, config.Btc.Indexer, btc.DefaultRetryInterval)
	btcWalletOptions := btcswap.NewWalletOptions(config.Btc.Chain.Params())
	btcWallet, err := btcswap.NewWallet(btcWalletOptions, indexer, util.EcdsaToBtcec(key), estimator)
	if err != nil {
		return Cobid{}, err
	}
	btcExe := executor.NewBitcoinExecutor(config.Btc.Chain, logger, btcWallet, client, storage, strings.ToLower(addr.Hex()))

	// Ethereum wallet and executor
	wallets := map[model.Chain]ethswap.Wallet{}
	clients := map[model.Chain]*ethclient.Client{}
	for _, evm := range config.Evms {
		ethClient, err := ethclient.Dial(evm.URL)
		if err != nil {
			return Cobid{}, err
		}

		swapAddr := common.HexToAddress(evm.SwapAddress)
		ethWalletOptions := ethswap.NewOptions(evm.Chain, swapAddr)
		ethWallet, err := ethswap.NewWallet(ethWalletOptions, key, ethClient)
		if err != nil {
			return Cobid{}, err
		}
		wallets[evm.Chain] = ethWallet
		clients[evm.Chain] = ethClient
	}
	dialer := func() rest.WSClient {
		return rest.NewWSClient(fmt.Sprintf("wss://%s/", config.OrderbookURL), logger)
	}
	ethExe := executor.NewEvmExecutor(logger, wallets, clients, storage, dialer)
	exes := executor.Executors{btcExe, ethExe}

	return Cobid{
		executors: exes,
		filler:    filler.New(config.Strategies, btcWallet, wallets, client, dialer, logger),
	}, nil
}

func (cb Cobid) Start() error {
	cb.executors.Start()
	return cb.filler.Start()
}

func (cb Cobid) Stop() {
	cb.executors.Stop()
	cb.filler.Stop()
}
