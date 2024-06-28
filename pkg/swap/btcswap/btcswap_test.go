package btcswap_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/catalogfi/blockchain/localnet"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/fatih/color"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bitcoin swap", func() {
	Context("Alice and Bob wants to swap 0.01 BTC for 0.02 BTC", func() {
		It("should succeed", func(ctx context.Context) {
			By("Initialization two wallet")
			aliceWallet, err := NewTestWallet(indexer)
			Expect(err).To(BeNil())
			bobWallet, err := NewTestWallet(indexer)
			Expect(err).To(BeNil())

			By("Funding the wallet")
			txhash1, err := localnet.FundBTC(aliceWallet.Address().EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding address1 %v , txid = %v", aliceWallet.Address(), txhash1))
			txhash2, err := localnet.FundBTC(bobWallet.Address().EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding address2 %v , txid = %v", aliceWallet.Address(), txhash2))
			time.Sleep(5 * time.Second)

			By("Alice and Bob construct their own swap")
			waitBlocks := int64(6)
			secret := localnet.RandomSecret()
			secretHash := sha256.Sum256(secret)
			aliceSwap, err := btcswap.NewSwap(network, aliceWallet.Address(), bobWallet.Address(), 1e7, secretHash[:], waitBlocks)
			Expect(err).To(BeNil())
			bobSwap, err := btcswap.NewSwap(network, bobWallet.Address(), aliceWallet.Address(), 2e7, secretHash[:], waitBlocks)
			Expect(err).To(BeNil())

			By("Check swap status")
			initiated, _, err := aliceSwap.Initiated(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			initiated, _, err = bobSwap.Initiated(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())

			By("Alice initiates her swap")
			initiatedTx, err := aliceWallet.Initiate(ctx, aliceSwap)
			Expect(err).To(BeNil())
			By(color.GreenString("Alice's swap is initiated in tx %v", initiatedTx))
			Expect(localnet.MineBTCBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)

			By("Check swap initiators")
			initiators, err := aliceSwap.Initiators(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(len(initiators)).Should(Equal(1))
			Expect(initiators[0]).Should(Equal(aliceWallet.Address().EncodeAddress()))

			By("Check swap status")
			latest, err := indexer.GetTipBlockHeight(ctx)
			Expect(err).To(BeNil())
			initiated, included, err := aliceSwap.Initiated(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			Expect(latest - included + 1).Should(Equal(uint64(1)))

			By("Bob initiates his swap")
			initiatedTx, err = bobWallet.Initiate(ctx, bobSwap)
			Expect(err).To(BeNil())
			By(color.GreenString("Bob's swap is initiated in tx %v", initiatedTx))
			Expect(localnet.MineBTCBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)

			By("Check swap initiators")
			initiators, err = bobSwap.Initiators(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(len(initiators)).Should(Equal(1))
			Expect(initiators[0]).Should(Equal(bobWallet.Address().EncodeAddress()))

			By("Check swap status")
			latest, err = indexer.GetTipBlockHeight(ctx)
			Expect(err).To(BeNil())
			initiated, included, err = bobSwap.Initiated(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			Expect(latest - included + 1).Should(Equal(uint64(1)))
			redeemed, _, err := aliceSwap.Redeemed(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())
			redeemed, _, err = bobSwap.Redeemed(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice redeems Bob's swap and reveal the secret")
			redeemTx, err := aliceWallet.Redeem(ctx, bobSwap, secret, aliceWallet.Address().EncodeAddress())
			Expect(err).Should(BeNil())
			By(color.GreenString("Bob's swap is redeemed in tx %v", redeemTx))
			Expect(localnet.MineBTCBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)
			redeemed, revealedSecret, err := bobSwap.Redeemed(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeTrue())
			Expect(bytes.Equal(secret, revealedSecret)).Should(BeTrue())

			By("Bob redeems Alice's swap using the revealed secret")
			redeemTx, err = bobWallet.Redeem(ctx, aliceSwap, revealedSecret, bobWallet.Address().EncodeAddress())
			Expect(err).Should(BeNil())
			By(color.GreenString("Alice's swap is redeemed in tx %v", redeemTx))
			Expect(localnet.MineBTCBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)
			redeemed, _, err = bobSwap.Redeemed(ctx, indexer)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeTrue())
		})
	})
})
