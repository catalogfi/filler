package btcswap_test

import (
	"math/rand"
	"os"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/blockchain/btc/btctest"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBtcswap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Btcswap Suite")
}

var _ = BeforeSuite(func() {
	// Check the ENVS are set for the tests.
	By("These are the requirements for all tests in this suite. ")
	By("You may want to disable some assertion when forcing running a specific test")
	// Expect(os.Getenv("BTC_INDEXER_ELECTRS_REGNET")).ShouldNot(BeEmpty())
})

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
