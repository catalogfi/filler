package utils

import (
	"net/http"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/pkg/blockchain"
	"github.com/catalogfi/cobi/pkg/swapper/bitcoin"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/guardian"
	"github.com/catalogfi/guardian/jsonrpc"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func LoadDB(dbDialector string) (store.Store, error) {
	var str store.Store
	var err error
	if dbDialector != "" {
		str, err = store.NewStore(sqlite.Open(dbDialector), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			return nil, err
		}
	} else {
		str, err = store.NewStore(sqlite.Open(DefaultStorePath()), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			return nil, err
		}
	}
	return str, nil
}
func LoadIwDB(dbDialector string) (bitcoin.Store, error) {
	var iwStore bitcoin.Store
	var err error
	if dbDialector != "" {
		iwStore, err = bitcoin.NewStore(sqlite.Open(dbDialector), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			return nil, err
		}
	} else {
		iwStore, err = bitcoin.NewStore((DefaultInstantWalletDBDialector()), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			return nil, err
		}
	}
	return iwStore, nil
}

func LoadKeys(mnemonic string) (Keys, error) {
	entropy, err := bip39.EntropyFromMnemonic(mnemonic)
	if err != nil {
		return Keys{}, err
	}

	return NewKeys(entropy), nil

}

func GetGuardianWallet(fromKeyInterface interface{}, logger *zap.Logger, chain model.Chain, network model.Network) (guardian.BitcoinWallet, error) {
	privKey := fromKeyInterface.(*btcec.PrivateKey)
	chainParams := blockchain.GetParams(chain)
	rpcClient := jsonrpc.NewClient(new(http.Client), network[chain].IWRPC)
	feeEstimator := btc.NewBlockstreamFeeEstimator(chainParams, network[chain].RPC["mempool"], 20*time.Second)
	indexer := btc.NewElectrsIndexerClient(logger, network[chain].RPC["mempool"], 5*time.Second)

	return guardian.NewBitcoinWallet(logger, privKey, chainParams, indexer, feeEstimator, rpcClient)
}
