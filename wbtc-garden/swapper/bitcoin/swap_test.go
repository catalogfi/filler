package bitcoin_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc"
	testutil2 "github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"github.com/catalogfi/guardian"
	"github.com/catalogfi/guardian/jsonrpc"
	"github.com/catalogfi/guardian/testutil"
	"github.com/fatih/color"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("atomic swap", func() {
	Describe("with default client", func() {
		Context("when Alice and Bob wants to trade BTC for BTC", func() {
			It("should work if both are honest player", func() {
				Equal(64)
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
				Expect(len(initHash1FromChain)).Should(Equal(64))
				Expect(initHash1).Should(Equal(initHash1FromChain))

				By("Follower initiate his swap")
				initHash2, err := followerInitSwap.Initiate()
				Expect(err).Should(BeNil())
				By(color.GreenString("Init follower's swap = %v", initHash2))

				By("Initiator wait for initiation is confirmed")
				_, err = NigiriFaucet(addr1.EncodeAddress()) // mine a block
				time.Sleep(5 * time.Second)
				initHash2FromChain, err := initiatorFollSwap.WaitForInitiate()
				Expect(err).Should(BeNil())
				Expect(len(initHash2FromChain)).Should(Equal(64))
				Expect(initHash2).Should(Equal(initHash2FromChain))

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
				Equal(64)
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
				Equal(64)
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
				Expect(len(initHash1FromChain)).Should(Equal(64))
				Expect(initHash1).Should(Equal(initHash1FromChain))

				By("Follower initiate his swap")
				initHash2, err := followerInitSwap.Initiate()
				Expect(err).Should(BeNil())
				By(color.GreenString("Init follower's swap = %v", initHash2))

				By("Initiator wait for initiation is confirmed")
				_, err = NigiriFaucet(addr1.EncodeAddress()) // mine a block
				time.Sleep(5 * time.Second)
				initHash2FromChain, err := initiatorFollSwap.WaitForInitiate()
				Expect(err).Should(BeNil())
				Expect(len(initHash2FromChain)).Should(Equal(64))
				Expect(initHash2).Should(Equal(initHash2FromChain))

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
				Equal(64)

			})

			It("test what happens with other types of address, P2WSH?", func() {
				Equal(64)

			})
		})
	})
	Describe("with gurdian iw client", func() {
		Context("when Alice and Bob wants to trade BTC for BTC", func() {
			It("should work if both are honest player", func() {
				pk1, err := btcec.NewPrivateKey()
				Expect(err).Should(BeNil())

				pk2, err := btcec.NewPrivateKey()
				Expect(err).Should(BeNil())
				By("Initialise client")
				db := "test.db"

				network := &chaincfg.RegressionNetParams

				logger, err := zap.NewDevelopment()
				Expect(err).Should(BeNil())

				By("fund wallet master address")
				DepositAddress1, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(pk1.PubKey().SerializeCompressed()), network)
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress1.String())
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress1.String())
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress1.String())
				Expect(err).Should(BeNil())

				DepositAddress2, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(pk2.PubKey().SerializeCompressed()), network)
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress2.String())
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress2.String())
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress2.String())
				Expect(err).Should(BeNil())

				_, btcIndexer, err := testutil.RegtestClient(logger)
				Expect(err).Should(BeNil())

				signerServerURL := "http://localhost:12345"
				rpcClient := jsonrpc.NewClient(new(http.Client), signerServerURL)
				feeEstimator := btc.NewFixFeeEstimator(20)
				guardianClient1, err := guardian.NewBitcoinWallet(logger, pk1, network, btcIndexer, feeEstimator, rpcClient)
				Expect(err).Should(BeNil())

				guardianClient2, err := guardian.NewBitcoinWallet(logger, pk2, network, btcIndexer, feeEstimator, rpcClient)
				Expect(err).Should(BeNil())

				electrs := "http://localhost:30000"
				defaultClient := bitcoin.NewClient(bitcoin.NewBlockstream(electrs), network)
				btcStore, err := bitcoin.NewStore(sqlite.Open(db))
				Expect(err).Should(BeNil())

				IWClient1 := bitcoin.InstantWalletWrapper(defaultClient, bitcoin.InstantWalletConfig{Store: btcStore, IWallet: guardianClient1})
				IWAddress1 := IWClient1.GetInstantWalletAddress()
				Expect(IWAddress1).ShouldNot(BeEmpty())

				IWClient2 := bitcoin.InstantWalletWrapper(defaultClient, bitcoin.InstantWalletConfig{Store: btcStore, IWallet: guardianClient2})
				IWAddress2 := IWClient2.GetInstantWalletAddress()
				Expect(IWAddress2).ShouldNot(BeEmpty())

				By("Create swaps")
				secret := RandomSecret()
				secretHash := sha256.Sum256(secret)
				waitBlock := int64(6)
				minConf := uint64(1)
				sendAmount, receiveAmount := uint64(1e8), uint64(1e7)
				initiatorInitSwap, err := bitcoin.NewInitiatorSwap(logger, pk1, DepositAddress2, secretHash[:], waitBlock, minConf, sendAmount, IWClient1)
				Expect(err).Should(BeNil())
				initiatorFollSwap, err := bitcoin.NewRedeemerSwap(logger, pk1, DepositAddress2, secretHash[:], waitBlock, minConf, receiveAmount, IWClient1)
				Expect(err).Should(BeNil())

				followerInitSwap, err := bitcoin.NewInitiatorSwap(logger, pk2, DepositAddress1, secretHash[:], waitBlock, minConf, sendAmount, IWClient2)
				Expect(err).Should(BeNil())
				followerFollSwap, err := bitcoin.NewRedeemerSwap(logger, pk2, DepositAddress1, secretHash[:], waitBlock, minConf, receiveAmount, IWClient2)
				Expect(err).Should(BeNil())

				By("Fund the coreesponding instant wallets")
				Expect(testutil2.NigiriNewBlock()).Should(Succeed())
				time.Sleep(5 * time.Second)
				txHash1, err := IWClient1.FundInstantWallet(pk1, int64(2*sendAmount))
				fmt.Println("funding txHash1:", txHash1)
				Expect(err).Should(BeNil())

				Expect(testutil2.NigiriNewBlock()).Should(Succeed())
				time.Sleep(5 * time.Second)
				txHash2, err := IWClient2.FundInstantWallet(pk2, int64(2*sendAmount))
				fmt.Println("funding txHash1:", txHash2)
				Expect(err).Should(BeNil())

				By("Initiator initiates the swap ")
				initHash1, err := initiatorInitSwap.Initiate()
				Expect(err).Should(BeNil())
				By(color.GreenString("Init initiator's swap = %v", initHash1))

				By("Follower wait for initiation is confirmed")
				_, err = NigiriFaucet(DepositAddress1.EncodeAddress()) // mine a block
				time.Sleep(5 * time.Second)
				initHash1FromChain, err := followerFollSwap.WaitForInitiate()
				Expect(err).Should(BeNil())
				Expect(len(initHash1FromChain)).Should(Equal(64))
				Expect(initHash1).Should(Equal(initHash1FromChain))

				By("Follower initiate his swap")
				initHash2, err := followerInitSwap.Initiate()
				Expect(err).Should(BeNil())
				By(color.GreenString("Init follower's swap = %v", initHash2))

				By("Initiator wait for initiation is confirmed")
				_, err = NigiriFaucet(DepositAddress1.EncodeAddress()) // mine a block
				time.Sleep(5 * time.Second)
				initHash2FromChain, err := initiatorFollSwap.WaitForInitiate()
				Expect(err).Should(BeNil())
				Expect(len(initHash2FromChain)).Should(Equal(64))
				Expect(initHash2).Should(Equal(initHash2FromChain))

				By("Initiator redeem follower's swap")
				_, err = NigiriFaucet(DepositAddress1.EncodeAddress()) // mine a block
				redeemHash1, err := initiatorFollSwap.Redeem(secret)
				Expect(err).Should(BeNil())
				By(color.GreenString("Redeem follower's swap = %v", redeemHash1))

				By("Follower waits for redeeming")
				_, err = NigiriFaucet(DepositAddress1.EncodeAddress())
				time.Sleep(5 * time.Second)
				secretFromChain, redeemTxid, err := followerInitSwap.WaitForRedeem()
				Expect(err).Should(BeNil())
				Expect(bytes.Equal(secretFromChain, secret)).Should(BeTrue())
				Expect(redeemTxid).Should(Equal(redeemHash1))

				By("Follower redeem initiator's swap")
				_, err = NigiriFaucet(DepositAddress1.EncodeAddress()) // mine a block
				redeemHash2, err := followerFollSwap.Redeem(secret)
				Expect(err).Should(BeNil())
				By(color.GreenString("Redeem initiator's swap = %v", redeemHash2))

				By("Fund the coreesponding instant wallets")
				Expect(testutil2.NigiriNewBlock()).Should(Succeed())
				time.Sleep(5 * time.Second)
				txHash3, err := IWClient1.FundInstantWallet(pk1, 200000)
				fmt.Println("funding txHash3:", txHash3)
				Expect(err).Should(BeNil())
			})
			It("should allow Alice to refund if Bob doesn't initiate", func() {
				pk1, err := btcec.NewPrivateKey()
				Expect(err).Should(BeNil())

				pk2, err := btcec.NewPrivateKey()
				Expect(err).Should(BeNil())

				network := &chaincfg.RegressionNetParams

				By("Initialise client")
				db := "test.db"

				logger, err := zap.NewDevelopment()
				Expect(err).Should(BeNil())

				By("fund wallet master address")
				DepositAddress1, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(pk1.PubKey().SerializeCompressed()), network)
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress1.String())
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress1.String())
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress1.String())
				Expect(err).Should(BeNil())

				DepositAddress2, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(pk2.PubKey().SerializeCompressed()), network)
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress2.String())
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress2.String())
				Expect(err).Should(BeNil())
				_, err = testutil2.NigiriFaucet(DepositAddress2.String())
				Expect(err).Should(BeNil())

				_, btcIndexer, err := testutil.RegtestClient(logger)
				Expect(err).Should(BeNil())

				signerServerURL := "http://localhost:12345"
				rpcClient := jsonrpc.NewClient(new(http.Client), signerServerURL)
				feeEstimator := btc.NewFixFeeEstimator(20)
				guardianClient1, err := guardian.NewBitcoinWallet(logger, pk1, network, btcIndexer, feeEstimator, rpcClient)
				Expect(err).Should(BeNil())

				guardianClient2, err := guardian.NewBitcoinWallet(logger, pk2, network, btcIndexer, feeEstimator, rpcClient)
				Expect(err).Should(BeNil())

				electrs := "http://localhost:30000"
				defaultClient := bitcoin.NewClient(bitcoin.NewBlockstream(electrs), network)
				btcStore, err := bitcoin.NewStore(sqlite.Open(db))
				Expect(err).Should(BeNil())

				IWClient1 := bitcoin.InstantWalletWrapper(defaultClient, bitcoin.InstantWalletConfig{Store: btcStore, IWallet: guardianClient1})
				IWAddress1 := IWClient1.GetInstantWalletAddress()
				Expect(IWAddress1).ShouldNot(BeEmpty())

				IWClient2 := bitcoin.InstantWalletWrapper(defaultClient, bitcoin.InstantWalletConfig{Store: btcStore, IWallet: guardianClient2})
				IWAddress2 := IWClient2.GetInstantWalletAddress()
				Expect(IWAddress2).ShouldNot(BeEmpty())

				By("Create swaps")
				secret := RandomSecret()
				secretHash := sha256.Sum256(secret)
				waitBlock := int64(6)
				minConf := uint64(1)
				sendAmount, _ := uint64(1e8), uint64(1e7)
				initiatorInitSwap, err := bitcoin.NewInitiatorSwap(logger, pk1, DepositAddress2, secretHash[:], waitBlock, minConf, sendAmount, IWClient1)
				Expect(err).Should(BeNil())
				// initiatorFollSwap, err := bitcoin.NewRedeemerSwap(logger, pk1, DepositAddress2, secretHash[:], waitBlock, minConf, receiveAmount, IWClient1)
				// Expect(err).Should(BeNil())

				// followerInitSwap, err := bitcoin.NewInitiatorSwap(logger, pk2, DepositAddress1, secretHash[:], waitBlock, minConf, sendAmount, IWClient2)
				// Expect(err).Should(BeNil())
				// followerFollSwap, err := bitcoin.NewRedeemerSwap(logger, pk2, DepositAddress1, secretHash[:], waitBlock, minConf, receiveAmount, IWClient2)
				// Expect(err).Should(BeNil())

				By("Fund the coreesponding instant wallets")
				Expect(testutil2.NigiriNewBlock()).Should(Succeed())
				time.Sleep(5 * time.Second)
				txHash1, err := IWClient1.FundInstantWallet(pk1, int64(2*sendAmount))
				fmt.Println("funding txHash1:", txHash1)
				Expect(err).Should(BeNil())

				Expect(testutil2.NigiriNewBlock()).Should(Succeed())
				time.Sleep(5 * time.Second)
				txHash2, err := IWClient2.FundInstantWallet(pk2, int64(2*sendAmount))
				fmt.Println("funding txHash1:", txHash2)
				Expect(err).Should(BeNil())

				By("Initiator initiates the swap ")
				initHash1, err := initiatorInitSwap.Initiate()
				Expect(err).Should(BeNil())
				By(color.GreenString("Init initiator's swap = %v", initHash1))

				By("Follower doesn't initiate")
				for i := int64(0); i <= waitBlock; i++ {
					NigiriFaucet(DepositAddress1.EncodeAddress())
				}
				time.Sleep(5 * time.Second)

				By("Initiator refund")
				refundTxid, err := initiatorInitSwap.Refund()
				Expect(err).Should(BeNil())
				By(color.GreenString("Refund tx hash %v", refundTxid))
			})

		})
	})
})
