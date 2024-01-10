package cobid

import (
	"encoding/hex"
	"fmt"

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

type Config struct {
	BtcChain     model.Chain
	EthChain     model.Chain
	Key          string
	BtcIndexer   string
	EthURL       string
	SwapAddress  string
	OrderbookURL string
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

	// Blockchain clients
	indexer := btc.NewElectrsIndexerClient(logger, config.BtcIndexer, btc.DefaultRetryInterval)
	ethClient, err := ethclient.Dial(config.EthURL)
	if err != nil {
		return Cobid{}, err
	}

	// Bitcoin wallet and executor
	btcWalletOptions := btcswap.NewWalletOptions(config.BtcChain.Params())
	btcWallet, err := btcswap.NewWallet(btcWalletOptions, indexer, util.EcdsaToBtcec(key), estimator)
	if err != nil {
		return Cobid{}, err
	}
	storage := executor.NewInMemStore()
	btcExe := executor.NewBitcoinExecutor(config.BtcChain, logger, btcWallet, storage)

	// Ethereum wallet and executor
	swapAddr := common.HexToAddress(config.SwapAddress)
	ethWalletOptions := ethswap.NewOptions(config.EthChain, swapAddr)
	ethWallet, err := ethswap.NewWallet(ethWalletOptions, key, ethClient)
	if err != nil {
		return Cobid{}, err
	}
	ethExe := executor.NewEvmExecutor(config.EthChain, logger, ethWallet, storage)
	if err != nil {
		return Cobid{}, err
	}

	// Executors
	addr := crypto.PubkeyToAddress(key.PublicKey)
	wsClient := rest.NewWSClient(fmt.Sprintf("wss://%s/", config.OrderbookURL), logger)
	exes := executor.New(logger, []executor.Executor{btcExe, ethExe}, addr.Hex(), wsClient, storage)

	// Filler
	client := rest.NewClient(fmt.Sprintf("https://%s", config.OrderbookURL), config.Key)
	token, err := client.Login()
	if err != nil {
		return Cobid{}, err
	}
	if err := client.SetJwt(token); err != nil {
		return Cobid{}, err
	}
	filler := filler.New(config.Strategies, client, config.OrderbookURL, logger)

	return Cobid{
		executors: exes,
		filler:    filler,
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
