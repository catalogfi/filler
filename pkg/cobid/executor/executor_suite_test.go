package executor_test

import (
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/blockchain/btc/btctest"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExecutor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Executor Suite")
}

func NewTestWallet(network *chaincfg.Params, client btc.IndexerClient) (btcswap.Wallet, error) {
	key, _, err := btctest.NewBtcKey(network)
	if err != nil {
		return nil, err
	}
	opts := btcswap.OptionsRegression()
	fee := rand.Intn(18) + 2
	feeEstimator := btc.NewFixFeeEstimator(fee)
	return btcswap.New(opts, client, key, feeEstimator)
}
