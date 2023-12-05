package swapper_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc/btctest"
	"github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/fatih/color"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Swap between different chains", func() {
	Context("Alice wants to swap 0.01 BTC for Bob's 1 ERC20", func() {
		It("should not return any error", func(ctx context.Context) {
			By("Initialize Alice's key and bitcoin client")
			network := &chaincfg.RegressionNetParams
			aliceKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_2"), "0x")
			aliceKeyBytes, err := hex.DecodeString(aliceKeyStr)
			Expect(err).To(BeNil())
			aliceKey, err := crypto.ToECDSA(aliceKeyBytes)
			Expect(err).To(BeNil())
			aliceEthAddr := crypto.PubkeyToAddress(aliceKey.PublicKey)
			aliceBtcKey, _ := btcec.PrivKeyFromBytes(aliceKeyBytes)
			aliceBtcAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(aliceBtcKey.PubKey().SerializeCompressed()), network)
			Expect(err).To(BeNil())
			btcClient := btctest.RegtestIndexer()

			By("Initialize Bob's key and ethereum client")
			bobKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
			bobKeyBytes, err := hex.DecodeString(bobKeyStr)
			Expect(err).To(BeNil())
			bobKey, err := crypto.ToECDSA(bobKeyBytes)
			Expect(err).To(BeNil())
			bobEthAddr := crypto.PubkeyToAddress(bobKey.PublicKey)
			bobBtcKey, _ := btcec.PrivKeyFromBytes(bobKeyBytes)
			bobBtcAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(bobBtcKey.PubKey().SerializeCompressed()), network)
			Expect(err).To(BeNil())

			By("Funding both user's bitcoin address")
			txhash1, err := testutil.NigiriFaucet(aliceBtcAddr.EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding address1 %v , txid = %v", aliceBtcAddr.EncodeAddress(), txhash1))
			txhash2, err := testutil.NigiriFaucet(bobBtcAddr.EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding address2 %v , txid = %v", bobBtcAddr.EncodeAddress(), txhash2))
			time.Sleep(5 * time.Second)

			url := os.Getenv("ETH_URL")
			ethClient, err := ethclient.Dial(url)
			Expect(err).To(BeNil())

			By("Alice constructs her swap on bitcoin side")
			amountBtc := int64(1e6)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitBlocks := int64(3)
			aliceSwap, err := btcswap.NewSwap(network, btcClient, aliceBtcAddr, bobBtcAddr, amountBtc, secretHash[:], waitBlocks)
			Expect(err).To(BeNil())

			By("Bob constructs his swap on ethereum side")
			amountErc20 := big.NewInt(1e18)
			expiry := big.NewInt(6)
			bobSwap, err := ethswap.NewSwap(bobEthAddr, aliceEthAddr, swapAddr, ethClient, secretHash, amountErc20, expiry)
			Expect(err).To(BeNil())

			By("Check swap status")
			initiated, _, err := aliceSwap.Initiated(ctx)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			initiated, err = bobSwap.Initiated()
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())

			By("Alice initiates the swap")
			feeRate := 5
			initiatedTx, err := aliceSwap.Initiate(ctx, aliceBtcKey, feeRate)
			Expect(err).To(BeNil())
			By(color.GreenString("Alice's swap is initiated in tx %v", initiatedTx))
			Expect(testutil.NigiriNewBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)

			By("Check swap status")
			latest, err := btcClient.GetTipBlockHeight(ctx)
			Expect(err).To(BeNil())
			initiated, included, err := aliceSwap.Initiated(ctx)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			Expect(latest - included + 1).Should(Equal(uint64(1)))

			By("Bob initiates his swap")
			initiatedTx, err = bobSwap.Initiate(ctx, bobKey)
			Expect(err).To(BeNil())
			By(color.GreenString("Bob's swap is initiated in tx %v", initiatedTx))
			time.Sleep(time.Second)
			initiated, err = bobSwap.Initiated()
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())

			By("Both swap should not been redeemed")
			redeemed, _, err := aliceSwap.Redeemed(ctx)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())
			redeemed, _, err = bobSwap.Redeemed(ctx, 0)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice redeems Bob's swap")
			redeemTx, err := bobSwap.Redeem(ctx, aliceKey, secret)
			Expect(err).To(BeNil())
			By(color.GreenString("Bob's swap is redeemed in tx %v", redeemTx))
			time.Sleep(time.Second)
			redeemed, revealedSecret, err := bobSwap.Redeemed(ctx, 0)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeTrue())
			Expect(bytes.Equal(revealedSecret, secret))

			By("Bob redeems Alice's swap using the revealed secret")
			redeemTx, err = aliceSwap.Redeem(ctx, bobBtcKey, revealedSecret, feeRate, bobBtcAddr.EncodeAddress())
			Expect(err).To(BeNil())
			By(color.GreenString("Alice's swap is redeemed in tx %v", redeemTx))
			Expect(testutil.NigiriNewBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)
			redeemed, revealedSecret, err = aliceSwap.Redeemed(ctx)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeTrue())
			Expect(bytes.Equal(revealedSecret, secret))
		})
	})

	Context("Alice refund after the swap is expired", func() {
		It("should refund the money to Alice", func(ctx context.Context) {
			By("Initialize Alice's key and bitcoin client")
			network := &chaincfg.RegressionNetParams
			aliceKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_2"), "0x")
			aliceKeyBytes, err := hex.DecodeString(aliceKeyStr)
			Expect(err).To(BeNil())
			aliceKey, err := crypto.ToECDSA(aliceKeyBytes)
			Expect(err).To(BeNil())
			aliceEthAddr := crypto.PubkeyToAddress(aliceKey.PublicKey)
			aliceBtcKey, _ := btcec.PrivKeyFromBytes(aliceKeyBytes)
			aliceBtcAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(aliceBtcKey.PubKey().SerializeCompressed()), network)
			Expect(err).To(BeNil())
			btcClient := btctest.RegtestIndexer()

			By("Initialize Bob's key and ethereum client")
			bobKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
			bobKeyBytes, err := hex.DecodeString(bobKeyStr)
			Expect(err).To(BeNil())
			bobKey, err := crypto.ToECDSA(bobKeyBytes)
			Expect(err).To(BeNil())
			bobEthAddr := crypto.PubkeyToAddress(bobKey.PublicKey)
			bobBtcKey, _ := btcec.PrivKeyFromBytes(bobKeyBytes)
			bobBtcAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(bobBtcKey.PubKey().SerializeCompressed()), network)
			Expect(err).To(BeNil())

			By("Funding both user's bitcoin address")
			txhash1, err := testutil.NigiriFaucet(aliceBtcAddr.EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding address1 %v , txid = %v", aliceBtcAddr.EncodeAddress(), txhash1))
			txhash2, err := testutil.NigiriFaucet(bobBtcAddr.EncodeAddress())
			Expect(err).To(BeNil())
			By(fmt.Sprintf("Funding address2 %v , txid = %v", bobBtcAddr.EncodeAddress(), txhash2))
			time.Sleep(5 * time.Second)

			url := os.Getenv("ETH_URL")
			ethClient, err := ethclient.Dial(url)
			Expect(err).To(BeNil())

			By("Alice constructs her swap on bitcoin side")
			amountBtc := int64(1e6)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitBlocks := int64(3)
			aliceSwap, err := btcswap.NewSwap(network, btcClient, aliceBtcAddr, bobBtcAddr, amountBtc, secretHash[:], waitBlocks)
			Expect(err).To(BeNil())

			By("Bob constructs his swap on ethereum side")
			amountErc20 := big.NewInt(1e18)
			expiry := big.NewInt(5)
			bobSwap, err := ethswap.NewSwap(bobEthAddr, aliceEthAddr, swapAddr, ethClient, secretHash, amountErc20, expiry)
			Expect(err).To(BeNil())

			By("Check swap status")
			initiated, _, err := aliceSwap.Initiated(ctx)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			initiated, err = bobSwap.Initiated()
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())

			By("Alice initiates the swap")
			feeRate := 5
			initiatedTx, err := aliceSwap.Initiate(ctx, aliceBtcKey, feeRate)
			Expect(err).To(BeNil())
			By(color.GreenString("Alice's swap is initiated in tx %v", initiatedTx))
			Expect(testutil.NigiriNewBlock()).Should(Succeed())
			time.Sleep(5 * time.Second)

			By("Check swap status")
			latest, err := btcClient.GetTipBlockHeight(ctx)
			Expect(err).To(BeNil())
			initiated, included, err := aliceSwap.Initiated(ctx)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			Expect(latest - included + 1).Should(Equal(uint64(1)))

			By("Bob initiates his swap")
			initiatedTx, err = bobSwap.Initiate(ctx, bobKey)
			Expect(err).To(BeNil())
			By(color.GreenString("Bob's swap is initiated in tx %v", initiatedTx))
			time.Sleep(time.Second)
			initiated, err = bobSwap.Initiated()
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())

			By("Both swap should not been redeemed")
			redeemed, _, err := aliceSwap.Redeemed(ctx)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())
			redeemed, _, err = bobSwap.Redeemed(ctx, 0)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Wait for a few blocks for both swap to expire")
			for i := int64(0); i < waitBlocks; i++ {
				Expect(testutil.NigiriNewBlock()).Should(Succeed())
			}
			time.Sleep(5 * time.Second)

			By("Alice refunds her swap")
			err = aliceSwap.Refund(ctx, aliceBtcKey, feeRate, aliceBtcAddr.EncodeAddress())
			Expect(err).Should(BeNil())

			By("Bob refunds his swap")
			err = bobSwap.Refund(ctx, bobKey)
			Expect(err).Should(BeNil())
		})
	})
})
