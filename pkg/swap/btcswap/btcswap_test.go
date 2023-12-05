package btcswap_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc/btctest"
	"github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/fatih/color"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bitcoin swap", func() {
	Context("Alice and Bob wants to swap 0.01 BTC", func() {
		It("should work without any error", func(ctx context.Context) {
			By("Initialization two keys")
			network := &chaincfg.RegressionNetParams
			aliceKey, _, err := btctest.NewBtcKey(network)
			Expect(err).To(BeNil())
			aliceAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(aliceKey.PubKey().SerializeCompressed()), network)
			Expect(err).To(BeNil())
			bobKey, _, err := btctest.NewBtcKey(network)
			Expect(err).To(BeNil())
			bobAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(bobKey.PubKey().SerializeCompressed()), network)
			Expect(err).To(BeNil())
			client := btctest.RegtestIndexer()

			By("Funding the addresses")
			txhash1, err := testutil.NigiriFaucet(aliceAddr.EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding address1 %v , txid = %v", aliceAddr.EncodeAddress(), txhash1))
			txhash2, err := testutil.NigiriFaucet(bobAddr.EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding address2 %v , txid = %v", bobAddr.EncodeAddress(), txhash2))
			time.Sleep(5 * time.Second)

			By("Alice and Bob construct their own swap")
			amount := int64(1e6)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitBlocks := int64(3)
			aliceSwap, err := btcswap.NewSwap(network, client, aliceAddr, bobAddr, amount, secretHash[:], waitBlocks)
			Expect(err).To(BeNil())
			bobSwap, err := btcswap.NewSwap(network, client, bobAddr, aliceAddr, amount, secretHash[:], waitBlocks)
			Expect(err).To(BeNil())

			By("Check swap status")
			initiated, _, err := aliceSwap.Initiated(ctx)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			initiated, _, err = bobSwap.Initiated(ctx)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())

			By("Alice initiates her swap")
			feeRate := 5
			initiatedTx, err := aliceSwap.Initiate(ctx, aliceKey, feeRate)
			Expect(err).To(BeNil())
			By(color.GreenString("Alice's swap is initiated in tx %v", initiatedTx))
			Expect(testutil.NigiriNewBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)

			By("Check swap status")
			latest, err := client.GetTipBlockHeight(ctx)
			Expect(err).To(BeNil())
			initiated, included, err := aliceSwap.Initiated(ctx)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			Expect(latest - included + 1).Should(Equal(uint64(1)))

			By("Bob initiates his swap")
			initiatedTx, err = bobSwap.Initiate(ctx, bobKey, feeRate)
			Expect(err).To(BeNil())
			By(color.GreenString("Bob's swap is initiated in tx %v", initiatedTx))
			Expect(testutil.NigiriNewBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)

			By("Check swap status")
			latest, err = client.GetTipBlockHeight(ctx)
			Expect(err).To(BeNil())
			initiated, included, err = bobSwap.Initiated(ctx)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			Expect(latest - included + 1).Should(Equal(uint64(1)))
			redeemed, _, err := aliceSwap.Redeemed(ctx)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())
			redeemed, _, err = bobSwap.Redeemed(ctx)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice redeems Bob's swap and reveal the secret")
			redeemTx, err := bobSwap.Redeem(ctx, aliceKey, secret, feeRate, aliceAddr.EncodeAddress())
			Expect(err).Should(BeNil())
			By(color.GreenString("Bob's swap is redeemed in tx %v", redeemTx))
			Expect(testutil.NigiriNewBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)
			redeemed, revealedSecret, err := bobSwap.Redeemed(ctx)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeTrue())
			Expect(bytes.Equal(secret, revealedSecret)).Should(BeTrue())

			By("Bob redeems Alice's swap using the revealed secret")
			redeemTx, err = aliceSwap.Redeem(ctx, bobKey, revealedSecret, feeRate, bobAddr.EncodeAddress())
			Expect(err).Should(BeNil())
			By(color.GreenString("Alice's swap is redeemed in tx %v", redeemTx))
			Expect(testutil.NigiriNewBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)
			redeemed, _, err = bobSwap.Redeemed(ctx)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeTrue())
		})
	})

	Context("Alice wants to refund her money after initiation", func() {
		It("should work without any error", func(ctx context.Context) {
			By("Initialization two keys")
			network := &chaincfg.RegressionNetParams
			aliceKey, _, err := btctest.NewBtcKey(network)
			Expect(err).To(BeNil())
			aliceAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(aliceKey.PubKey().SerializeCompressed()), network)
			Expect(err).To(BeNil())
			bobKey, _, err := btctest.NewBtcKey(network)
			// _, _, err := btctest.NewBtcKey(network)
			Expect(err).To(BeNil())
			bobAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(bobKey.PubKey().SerializeCompressed()), network)
			Expect(err).To(BeNil())
			client := btctest.RegtestIndexer()

			By("Funding Alice's address")
			txhash1, err := testutil.NigiriFaucet(aliceAddr.EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding address1 %v , txid = %v", aliceAddr.EncodeAddress(), txhash1))
			time.Sleep(5 * time.Second)

			By("Alice constructs a new swap")
			amount := int64(1e6)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitBlocks := int64(3)
			aliceSwap, err := btcswap.NewSwap(network, client, aliceAddr, bobAddr, amount, secretHash[:], waitBlocks)
			Expect(err).To(BeNil())

			By("Alice initiates her swap")
			feeRate := 5
			initiatedTx, err := aliceSwap.Initiate(ctx, aliceKey, feeRate)
			Expect(err).To(BeNil())
			By(color.GreenString("Alice's swap is initiated in tx %v", initiatedTx))

			By("Wait for a few blocks to be mined")
			for i := int64(0); i < waitBlocks; i++ {
				Expect(testutil.NigiriNewBlock()).Should(Succeed())
			}
			time.Sleep(5 * time.Second)

			By("Alice tries to refund her money")
			Expect(aliceSwap.Refund(ctx, aliceKey, feeRate, aliceAddr.EncodeAddress())).Should(Succeed())
		})
	})
})
