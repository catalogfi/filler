package swap_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/blockchain/localnet"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/fatih/color"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Swap between different chains", func() {
	Context("Alice wants to swap 0.01 BTC for Bob's 1 ERC20", func() {
		It("should not return any error", func(ctx context.Context) {
			By("Initialise clients")
			url := os.Getenv("ETH_URL")
			ethClient, err := ethclient.Dial(url)
			Expect(err).To(BeNil())
			network := &chaincfg.RegressionNetParams

			By("Initialize Alice's wallets")
			aliceBtcKey, aliceKey, err := ParseKeyFromEnv("ETH_KEY_2")
			Expect(err).To(BeNil())
			aliceBtcWallet, err := btcswap.NewWallet(btcswap.OptionsRegression(), indexer, aliceBtcKey, btc.NewFixFeeEstimator(rand.Intn(18)+2))
			Expect(err).To(BeNil())
			aliceEthWallet, err := ethswap.NewWallet(ethswap.NewOptions(model.EthereumLocalnet, swapAddr), aliceKey, ethClient)
			Expect(err).To(BeNil())

			By("Initialize Bob's wallets ")
			bobBtcKey, bobKey, err := ParseKeyFromEnv("ETH_KEY_1")
			Expect(err).To(BeNil())
			bobBtcWallet, err := btcswap.NewWallet(btcswap.OptionsRegression(), indexer, bobBtcKey, btc.NewFixFeeEstimator(rand.Intn(18)+2))
			Expect(err).To(BeNil())
			bobEthWallet, err := ethswap.NewWallet(ethswap.NewOptions(model.EthereumLocalnet, swapAddr), bobKey, ethClient)
			Expect(err).To(BeNil())

			By("Funding both user's bitcoin address")
			txhash1, err := localnet.FundBTC(aliceBtcWallet.Address().EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding Alice's address %v , txid = %v", aliceBtcWallet.Address().EncodeAddress(), txhash1))
			txhash2, err := localnet.FundBTC(bobBtcWallet.Address().EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding Bob's address  %v , txid = %v", bobBtcWallet.Address().EncodeAddress(), txhash2))
			time.Sleep(5 * time.Second)

			By("Alice constructs her swap on bitcoin side")
			amountBtc := int64(1e6)
			secret := localnet.RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitBlocks := int64(3)
			aliceSwap, err := btcswap.NewSwap(network, aliceBtcWallet.Address(), bobBtcWallet.Address(), amountBtc, secretHash[:], waitBlocks)
			Expect(err).To(BeNil())

			By("Bob constructs his swap on ethereum side")
			amountErc20 := big.NewInt(1e18)
			expiry := big.NewInt(6)
			bobSwap := ethswap.NewSwap(bobEthWallet.Address(), aliceEthWallet.Address(), swapAddr, secretHash, amountErc20, expiry)

			By("Check swap status")
			initiated, _, err := aliceSwap.Initiated(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			initiated, err = bobSwap.Initiated(ctx, ethClient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())

			By("Alice initiates the swap")
			initiatedTxAlice, err := aliceBtcWallet.Initiate(ctx, aliceSwap)
			Expect(err).To(BeNil())
			By(color.GreenString("Alice's swap is initiated in tx %v", initiatedTxAlice))
			Expect(localnet.MineBTCBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)

			By("Check swap status")
			latest, err := indexer.GetTipBlockHeight(ctx)
			Expect(err).To(BeNil())
			initiated, included, err := aliceSwap.Initiated(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			Expect(latest - included + 1).Should(Equal(uint64(1)))

			By("Bob initiates his swap")
			initiatedTxBob, err := bobEthWallet.Initiate(ctx, bobSwap)
			Expect(err).To(BeNil())
			By(color.GreenString("Bob's swap is initiated in tx %v", initiatedTxBob.Hash().Hex()))
			time.Sleep(time.Second)
			initiated, err = bobSwap.Initiated(ctx, ethClient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())

			By("Both swap should not been redeemed")
			redeemed, _, err := aliceSwap.Redeemed(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())
			redeemed, err = bobSwap.Redeemed(ctx, ethClient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice redeems Bob's swap")
			redeemTxAlice, err := aliceEthWallet.Redeem(ctx, bobSwap, secret)
			Expect(err).To(BeNil())
			By(color.GreenString("Bob's swap is redeemed in tx %v", redeemTxAlice.Hash().Hex()))
			time.Sleep(time.Second)
			redeemed, err = bobSwap.Redeemed(ctx, ethClient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeTrue())
			revealedSecret, err := bobSwap.Secret(ctx, ethClient, 500)
			Expect(bytes.Equal(revealedSecret, secret))

			By("Bob redeems Alice's swap using the revealed secret")
			redeemTxBob, err := bobBtcWallet.Redeem(ctx, aliceSwap, revealedSecret, aliceBtcWallet.Address().EncodeAddress())
			Expect(err).To(BeNil())
			By(color.GreenString("Alice's swap is redeemed in tx %v", redeemTxBob))
			Expect(localnet.MineBTCBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)
			redeemed, revealedSecret, err = aliceSwap.Redeemed(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeTrue())
			Expect(bytes.Equal(revealedSecret, secret))
		})
	})

	Context("Alice refund after the swap is expired", func() {
		It("should refund the money to Alice", func(ctx context.Context) {
			By("Initialise clients")
			url := os.Getenv("ETH_URL")
			ethClient, err := ethclient.Dial(url)
			Expect(err).To(BeNil())
			network := &chaincfg.RegressionNetParams

			By("Initialize Alice's wallets")
			aliceBtcKey, aliceKey, err := ParseKeyFromEnv("ETH_KEY_2")
			Expect(err).To(BeNil())
			aliceBtcWallet, err := btcswap.NewWallet(btcswap.OptionsRegression(), indexer, aliceBtcKey, btc.NewFixFeeEstimator(rand.Intn(18)+2))
			Expect(err).To(BeNil())
			aliceEthWallet, err := ethswap.NewWallet(ethswap.NewOptions(model.EthereumLocalnet, swapAddr), aliceKey, ethClient)
			Expect(err).To(BeNil())

			By("Initialize Bob's wallets ")
			bobBtcKey, bobKey, err := ParseKeyFromEnv("ETH_KEY_1")
			Expect(err).To(BeNil())
			bobBtcWallet, err := btcswap.NewWallet(btcswap.OptionsRegression(), indexer, bobBtcKey, btc.NewFixFeeEstimator(rand.Intn(18)+2))
			Expect(err).To(BeNil())
			bobEthWallet, err := ethswap.NewWallet(ethswap.NewOptions(model.EthereumLocalnet, swapAddr), bobKey, ethClient)
			Expect(err).To(BeNil())

			By("Funding both user's bitcoin address")
			txhash1, err := localnet.FundBTC(aliceBtcWallet.Address().EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding Alice's address %v , txid = %v", aliceBtcWallet.Address().EncodeAddress(), txhash1))
			txhash2, err := localnet.FundBTC(bobBtcWallet.Address().EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding Bob's address  %v , txid = %v", bobBtcWallet.Address().EncodeAddress(), txhash2))
			time.Sleep(5 * time.Second)

			By("Alice constructs her swap on bitcoin side")
			amountBtc := int64(1e6)
			secret := localnet.RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitBlocks := int64(3)
			aliceSwap, err := btcswap.NewSwap(network, aliceBtcWallet.Address(), bobBtcWallet.Address(), amountBtc, secretHash[:], waitBlocks)
			Expect(err).To(BeNil())

			By("Bob constructs his swap on ethereum side")
			amountErc20 := big.NewInt(1e18)
			expiry := big.NewInt(3)
			bobSwap := ethswap.NewSwap(bobEthWallet.Address(), aliceEthWallet.Address(), swapAddr, secretHash, amountErc20, expiry)

			By("Alice initiates the swap")
			initiatedTxAlice, err := aliceBtcWallet.Initiate(ctx, aliceSwap)
			Expect(err).To(BeNil())
			By(color.GreenString("Alice's swap is initiated in tx %v", initiatedTxAlice))
			Expect(localnet.MineBTCBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)

			By("Bob initiates his swap")
			initiatedTxBob, err := bobEthWallet.Initiate(ctx, bobSwap)
			Expect(err).To(BeNil())
			By(color.GreenString("Bob's swap is initiated in tx %v", initiatedTxBob.Hash().Hex()))
			time.Sleep(time.Second)

			By("Wait for a few blocks for both swap to expire")
			for i := int64(0); i < waitBlocks; i++ {
				Expect(localnet.MineBTCBlock()).Should(Succeed())
			}
			time.Sleep(5 * time.Second)

			By("Alice refunds her swap")
			refundTxAlice, err := aliceBtcWallet.Refund(ctx, aliceSwap, aliceBtcWallet.Address().EncodeAddress())
			Expect(err).Should(BeNil())
			By(color.GreenString("Alice's swap is refunded in tx %v", refundTxAlice))

			By("Bob refunds his swap")
			refundTxBob, err := bobEthWallet.Refund(ctx, bobSwap)
			Expect(err).Should(BeNil())
			By(color.GreenString("Bob's swap is refunded in tx %v", refundTxBob.Hash().Hex()))
		})
	})
})
