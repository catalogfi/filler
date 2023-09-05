package bitcoin_test

import (
	"bytes"
	"crypto/sha256"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"github.com/fatih/color"
	"go.uber.org/zap"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("atomic swap", func() {
	Context("when Alice and Bob wants to trade BTC for BTC", func() {
		It("should work if both are honest player", func() {
			By("Initialise client")
			network := &chaincfg.RegressionNetParams
			electrs := "http://localhost:30000"
			client := bitcoin.NewClient(bitcoin.NewBlockstream(electrs), network)
			logger, err := zap.NewDevelopment()
			Expect(err).To(BeNil())

			By("Parse keys")
			pk1, addr1, err := ParseKey(PrivateKey1, network)
			Expect(err).Should(BeNil())
			pk2, addr2, err := ParseKey(PrivateKey2, network)
			Expect(err).Should(BeNil())

			By("Create swaps")
			secret := RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitBlock := int64(6)
			minConf := uint64(1)
			sendAmount, receiveAmount := uint64(1e8), uint64(1e7)
			initiatorInitSwap, err := bitcoin.NewInitiatorSwap(logger, pk1, addr2, secretHash[:], waitBlock, minConf, sendAmount, client)
			Expect(err).Should(BeNil())
			initiatorFollSwap, err := bitcoin.NewRedeemerSwap(logger, pk1, addr2, secretHash[:], waitBlock, minConf, receiveAmount, client)
			Expect(err).Should(BeNil())

			followerInitSwap, err := bitcoin.NewInitiatorSwap(logger, pk2, addr1, secretHash[:], waitBlock, minConf, sendAmount, client)
			Expect(err).Should(BeNil())
			followerFollSwap, err := bitcoin.NewRedeemerSwap(logger, pk2, addr1, secretHash[:], waitBlock, minConf, receiveAmount, client)
			Expect(err).Should(BeNil())

			By("Fund the wallets")
			_, err = NigiriFaucet(addr1.EncodeAddress())
			Expect(err).Should(BeNil())
			_, err = NigiriFaucet(addr2.EncodeAddress())
			Expect(err).Should(BeNil())
			time.Sleep(5 * time.Second)

			By("Initiator initiates the swap ")
			initHash1, err := initiatorInitSwap.Initiate()
			Expect(err).Should(BeNil())
			By(color.GreenString("Init initiator's swap = %v", initHash1))

			By("Follower wait for initiation is confirmed")
			_, err = NigiriFaucet(addr1.EncodeAddress()) // mine a block
			time.Sleep(5 * time.Second)
			initHash1FromChain, err := followerFollSwap.WaitForInitiate()
			Expect(err).Should(BeNil())
			Expect(len(initHash1FromChain)).Should(Equal(1))
			Expect(initHash1).Should(Equal(initHash1FromChain[0]))

			By("Follower initiate his swap")
			initHash2, err := followerInitSwap.Initiate()
			Expect(err).Should(BeNil())
			By(color.GreenString("Init follower's swap = %v", initHash2))

			By("Initiator wait for initiation is confirmed")
			_, err = NigiriFaucet(addr1.EncodeAddress()) // mine a block
			time.Sleep(5 * time.Second)
			initHash2FromChain, err := initiatorFollSwap.WaitForInitiate()
			Expect(err).Should(BeNil())
			Expect(len(initHash2FromChain)).Should(Equal(1))
			Expect(initHash2).Should(Equal(initHash2FromChain[0]))

			By("Initiator redeem follower's swap")
			_, err = NigiriFaucet(addr1.EncodeAddress()) // mine a block
			redeemHash1, err := initiatorFollSwap.Redeem(secret)
			Expect(err).Should(BeNil())
			By(color.GreenString("Redeem follower's swap = %v", redeemHash1))

			By("Follower waits for redeeming")
			_, err = NigiriFaucet(addr1.EncodeAddress())
			time.Sleep(5 * time.Second)
			secretFromChain, redeemTxid, err := followerInitSwap.WaitForRedeem()
			Expect(err).Should(BeNil())
			Expect(bytes.Equal(secretFromChain, secret)).Should(BeTrue())
			Expect(redeemTxid).Should(Equal(redeemHash1))

			By("Follower redeem initiator's swap")
			_, err = NigiriFaucet(addr1.EncodeAddress()) // mine a block
			redeemHash2, err := followerFollSwap.Redeem(secret)
			Expect(err).Should(BeNil())
			By(color.GreenString("Redeem initiator's swap = %v", redeemHash2))
		})

		It("should allow Alice to refund if Bob doesn't initiate", func() {
			By("Initialise client")
			network := &chaincfg.RegressionNetParams
			electrs := "http://localhost:30000"
			client := bitcoin.NewClient(bitcoin.NewBlockstream(electrs), network)
			logger, err := zap.NewDevelopment()
			Expect(err).To(BeNil())

			By("Parse keys")
			pk1, addr1, err := ParseKey(PrivateKey1, network)
			Expect(err).Should(BeNil())
			_, addr2, err := ParseKey(PrivateKey2, network)
			Expect(err).Should(BeNil())

			By("Create swaps")
			secret := RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitBlock := int64(6)
			minConf := uint64(1)
			sendAmount := uint64(1e8)
			initiatorInitSwap, err := bitcoin.NewInitiatorSwap(logger, pk1, addr2, secretHash[:], waitBlock, minConf, sendAmount, client)
			Expect(err).Should(BeNil())

			By("Fund the wallets")
			_, err = NigiriFaucet(addr1.EncodeAddress())
			Expect(err).Should(BeNil())
			_, err = NigiriFaucet(addr2.EncodeAddress())
			Expect(err).Should(BeNil())
			time.Sleep(5 * time.Second)

			By("Initiator initiates the swap ")
			initHash1, err := initiatorInitSwap.Initiate()
			Expect(err).Should(BeNil())
			By(color.GreenString("Init initiator's swap = %v", initHash1))

			By("Follower doesn't initiate")
			for i := int64(0); i <= waitBlock; i++ {
				_, err = NigiriFaucet(addr1.EncodeAddress())
			}
			time.Sleep(5 * time.Second)

			By("Initiator refund")
			refundTxid, err := initiatorInitSwap.Refund()
			Expect(err).Should(BeNil())
			By(color.GreenString("Refund tx hash %v", refundTxid))
		})

		It("should allow Bob to refund if Alice doesn't redeem", func() {
			By("Initialise client")
			network := &chaincfg.RegressionNetParams
			electrs := "http://localhost:30000"
			client := bitcoin.NewClient(bitcoin.NewBlockstream(electrs), network)
			logger, err := zap.NewDevelopment()
			Expect(err).To(BeNil())

			By("Parse keys")
			pk1, addr1, err := ParseKey(PrivateKey1, network)
			Expect(err).Should(BeNil())
			pk2, addr2, err := ParseKey(PrivateKey2, network)
			Expect(err).Should(BeNil())

			By("Create swaps")
			secret := RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitBlock := int64(6)
			minConf := uint64(1)
			sendAmount, receiveAmount := uint64(1e8), uint64(1e7)
			initiatorInitSwap, err := bitcoin.NewInitiatorSwap(logger, pk1, addr2, secretHash[:], waitBlock, minConf, sendAmount, client)
			Expect(err).Should(BeNil())
			initiatorFollSwap, err := bitcoin.NewRedeemerSwap(logger, pk1, addr2, secretHash[:], waitBlock, minConf, receiveAmount, client)
			Expect(err).Should(BeNil())

			followerInitSwap, err := bitcoin.NewInitiatorSwap(logger, pk2, addr1, secretHash[:], waitBlock, minConf, sendAmount, client)
			Expect(err).Should(BeNil())
			followerFollSwap, err := bitcoin.NewRedeemerSwap(logger, pk2, addr1, secretHash[:], waitBlock, minConf, receiveAmount, client)
			Expect(err).Should(BeNil())

			By("Fund the wallets")
			_, err = NigiriFaucet(addr1.EncodeAddress())
			Expect(err).Should(BeNil())
			_, err = NigiriFaucet(addr2.EncodeAddress())
			Expect(err).Should(BeNil())
			time.Sleep(5 * time.Second)

			By("Initiator initiates the swap ")
			initHash1, err := initiatorInitSwap.Initiate()
			Expect(err).Should(BeNil())
			By(color.GreenString("Init initiator's swap = %v", initHash1))

			By("Follower wait for initiation is confirmed")
			_, err = NigiriFaucet(addr1.EncodeAddress()) // mine a block
			time.Sleep(5 * time.Second)
			initHash1FromChain, err := followerFollSwap.WaitForInitiate()
			Expect(err).Should(BeNil())
			Expect(len(initHash1FromChain)).Should(Equal(1))
			Expect(initHash1).Should(Equal(initHash1FromChain[0]))

			By("Follower initiate his swap")
			initHash2, err := followerInitSwap.Initiate()
			Expect(err).Should(BeNil())
			By(color.GreenString("Init follower's swap = %v", initHash2))

			By("Initiator wait for initiation is confirmed")
			_, err = NigiriFaucet(addr1.EncodeAddress()) // mine a block
			time.Sleep(5 * time.Second)
			initHash2FromChain, err := initiatorFollSwap.WaitForInitiate()
			Expect(err).Should(BeNil())
			Expect(len(initHash2FromChain)).Should(Equal(1))
			Expect(initHash2).Should(Equal(initHash2FromChain[0]))

			By("Initiator doesn't redeem")
			for i := int64(0); i <= waitBlock; i++ {
				_, err = NigiriFaucet(addr1.EncodeAddress())
			}
			time.Sleep(5 * time.Second)

			By("Follower redeem")
			refundTxid, err := followerInitSwap.Refund()
			Expect(err).Should(BeNil())
			By(color.GreenString("Refund tx hash %v", refundTxid))
		})

		It("should not allow Alice/Bob to refund if timelock is not expired", func() {

		})

		It("test what happens with other types of address, P2WSH?", func() {

		})
	})
})
