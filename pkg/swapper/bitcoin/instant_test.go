package bitcoin_test

import (
	"fmt"
	"net/http"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc"
	testutil2 "github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/pkg/swapper/bitcoin"
	"github.com/catalogfi/guardian"
	"github.com/catalogfi/guardian/jsonrpc"
	"github.com/catalogfi/guardian/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
)

var _ = Describe("atomic swap", func() {

	Context("when Alice and Bob wants to trade BTC for BTC", Ordered, func() {
		var IWClient bitcoin.InstantClient
		var IWAddress string
		var DepositAddress btcutil.Address
		var PrivKey *btcec.PrivateKey
		BeforeAll(func() {
			db := ""
			logger, err := zap.NewDevelopment()
			Expect(err).Should(BeNil())

			network := &chaincfg.RegressionNetParams
			PrivKey, err = btcec.NewPrivateKey()
			Expect(err).Should(BeNil())

			DepositAddress, err = btcutil.NewAddressPubKeyHash(btcutil.Hash160(PrivKey.PubKey().SerializeCompressed()), network)
			Expect(err).Should(BeNil())
			_, err = testutil2.NigiriFaucet(DepositAddress.String())
			Expect(err).Should(BeNil())

			_, btcIndexer, err := testutil.RegtestClient(logger)
			Expect(err).Should(BeNil())

			signerServerURL := "http://localhost:12345"
			rpcClient := jsonrpc.NewClient(new(http.Client), signerServerURL)
			feeEstimator := btc.NewFixFeeEstimator(20)
			guardianClient, err := guardian.NewBitcoinWallet(logger, PrivKey, network, btcIndexer, feeEstimator, rpcClient)
			Expect(err).Should(BeNil())

			electrs := "http://localhost:30000"
			client := bitcoin.NewClient(bitcoin.NewBlockstream(electrs), network)
			btcStore, err := bitcoin.NewStore(postgres.Open(db))
			Expect(err).Should(BeNil())

			IWClient = bitcoin.InstantWalletWrapper(client, bitcoin.InstantWalletConfig{Store: btcStore, IWallet: guardianClient})
			IWAddress = IWClient.GetInstantWalletAddress()
			Expect(IWAddress).ShouldNot(BeEmpty())
		})
		It("should be able to deposit funds to empty instant wallet", func() {
			Expect(testutil2.NigiriNewBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)
			txHash, err := IWClient.FundInstantWallet(PrivKey, 200000)
			fmt.Println(txHash)
			Expect(err).Should(BeNil())

		})

		It("should be able to deposit funds to a funded instant wallet ", func() {
			Skip("failing with `failed to deposit , error : request failed : code = -32602, err = wallet not ready`")
			Expect(testutil2.NigiriNewBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)
			txHash, err := IWClient.FundInstantWallet(PrivKey, 200000)
			fmt.Println("funding txHash:", txHash)
			Expect(err).Should(BeNil())

		})

		It("should be able to send funds in instant wallet", func() {
			Skip("failing with `failed to deposit , error : request failed : code = -32602, err = wallet not ready`")
			txHash, err := IWClient.Send(DepositAddress, 20000, PrivKey)
			fmt.Println("sending txHash:", txHash)
			Expect(err).Should(BeNil())
		})

		It("should be able to spend funds and deposit ", func() {
			Skip("")
		})

		It("should not be able to spend more funds than balance", func() {
			Skip("")
		})

		It("sholuld not be able to spend funds from a failing swap and deposit", func() {
			//failure management
			Skip("")
		})
	})
})
