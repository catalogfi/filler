package btcswap_test

import (
	"math/rand"
	"os"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/blockchain/localnet"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"go.uber.org/zap"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	// Envs
	indexerHost string

	// Vars
	network *chaincfg.Params
	logger  *zap.Logger
	indexer btc.IndexerClient
)

func TestBtc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Orderbook Suite")
}

var _ = BeforeSuite(func() {
	By("Check if required ENVs are set.")
	By("You may want to disable some assertion when forcing running a specific test.")
	var ok bool
	indexerHost, ok = os.LookupEnv("BTC_REGNET_INDEXER")
	Expect(ok).Should(BeTrue())

	By("Initialise some variables used across tests")
	var err error
	network = &chaincfg.RegressionNetParams
	logger, err = zap.NewDevelopment()
	Expect(err).Should(BeNil())
	indexer = btc.NewElectrsIndexerClient(logger, indexerHost, btc.DefaultRetryInterval)
})

func NewTestWallet(client btc.IndexerClient) (btcswap.Wallet, error) {
	key, _, err := localnet.NewBtcKey(network, waddrmgr.WitnessPubKey)
	if err != nil {
		return nil, err
	}
	opts := btcswap.OptionsRegression()
	fee := rand.Intn(10) + 2
	feeEstimator := btc.NewFixFeeEstimator(fee)
	return btcswap.NewWallet(opts, client, key, feeEstimator)
}
