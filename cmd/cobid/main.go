package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"syscall"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/pkg/cobid"
	"github.com/catalogfi/cobi/pkg/cobid/filler"
	"github.com/catalogfi/cobi/pkg/util"
	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	btcChain, ethChain, err := ParseNetwork()
	if err != nil {
		panic(err)
	}
	config := cobid.Config{
		BtcChain:     btcChain,
		EthChain:     ethChain,
		Key:          parseRequiredEnv("PRIVATE_KEY"),
		BtcIndexer:   parseRequiredEnv("BITCOIN_INDEXER"),
		EthURL:       parseRequiredEnv("ETHEREUM_URL"),
		SwapAddress:  parseRequiredEnv("SWAP_CONTRACT"),
		OrderbookURL: parseRequiredEnv("ORDERBOOK_URL"),
	}

	// Decode key
	keyBytes, err := hex.DecodeString(config.Key)
	if err != nil {
		panic(err)
	}
	key, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		panic(err)
	}
	ethAddr := crypto.PubkeyToAddress(key.PublicKey)
	keyBytesHash := btcutil.Hash160(util.EcdsaToBtcec(key).PubKey().SerializeCompressed())
	btcAdr, err := btcutil.NewAddressWitnessPubKeyHash(keyBytesHash, btcChain.Params())
	if err != nil {
		panic(err)
	}
	config.Strategies = TestnetStrategies(ethAddr, btcAdr)

	estimator := btc.NewMempoolFeeEstimator(btcChain.Params(), btc.MempoolFeeAPI, btc.DefaultRetryInterval)
	cobi, err := cobid.NewCobi(config, estimator)
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
			OrderPair:      "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF",
			SendAddress:    ethAddr.Hex(),
			ReceiveAddress: btcAddr.EncodeAddress(),
			Makers:         nil,
			MinAmount:      big.NewInt(100000),
			MaxAmount:      big.NewInt(100000000),
			Fee:            10,
		},
		{
			OrderPair:      "ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF-bitcoin_testnet",
			SendAddress:    btcAddr.EncodeAddress(),
			ReceiveAddress: ethAddr.Hex(),
			Makers:         nil,
			MinAmount:      big.NewInt(100000),
			MaxAmount:      big.NewInt(100000000),
			Fee:            10,
		},
	}
}
