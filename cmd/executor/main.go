package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/pkg/cobid/executor"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/util"
	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
)

func main() {
	network, chain := ParseNetwork()
	host := testutil.ParseStringEnv("BITCOIN_INDEXER", "")
	keyStr := testutil.ParseStringEnv("PRIVATE_KEY", "")
	orderbook := testutil.ParseStringEnv("ORDERBOOK_URL", "")

	// Decode key
	keyBytes, err := hex.DecodeString(keyStr)
	if err != nil {
		panic(err)
	}
	key, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		panic(err)
	}
	logger, _ := zap.NewDevelopment()

	// Bitcoin wallet and executor
	indexer := btc.NewElectrsIndexerClient(logger, host, btc.DefaultRetryInterval)
	btcWalletOptions := btcswap.NewWalletOptions(network)
	estimator := btc.NewMempoolFeeEstimator(network, btc.MempoolFeeAPI, 15*time.Second)
	btcWallet, err := btcswap.NewWallet(btcWalletOptions, indexer, util.EcdsaToBtcec(key), estimator)
	if err != nil {
		panic(err)
	}
	storage := executor.NewInMemStore()
	btcExe := executor.NewBitcoinExecutor(chain, logger, btcWallet, storage, indexer)
	executors := []executor.Executor{btcExe}

	// Construct the executor and start
	signer := crypto.PubkeyToAddress(key.PublicKey)
	keyBytesHash := btcutil.Hash160(util.EcdsaToBtcec(key).PubKey().SerializeCompressed())
	btcAddr, err := btcutil.NewAddressWitnessPubKeyHash(keyBytesHash, network)
	if err != nil {
		panic(err)
	}
	log.Print("signer addr = ", signer.Hex())
	log.Print("btc    addr = ", btcAddr.EncodeAddress())
	exes := executor.New(logger, executors, signer.Hex(), storage, orderbook)
	exes.Start()
	defer exes.Stop()

	// waiting system signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGKILL)
	<-sigs
}

func ParseNetwork() (*chaincfg.Params, model.Chain) {
	network := os.Getenv("NETWORK")
	switch network {
	case "mainnet":
		return &chaincfg.MainNetParams, model.Bitcoin
	case "testnet":
		return &chaincfg.TestNet3Params, model.BitcoinTestnet
	default:
		panic(fmt.Errorf("unknown network = %v", network))
	}
}
