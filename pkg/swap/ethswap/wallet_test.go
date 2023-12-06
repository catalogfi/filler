package ethswap_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/fatih/color"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ethereum Atomic Swap", func() {
	Context("Alice and Bob wants to do a swap", func() {
		It("should work", func(ctx context.Context) {
			By("Initialise the client")
			url := os.Getenv("ETH_URL")
			client, err := ethclient.Dial(url)
			Expect(err).To(BeNil())

			By("Initialization two keys")
			aliceKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
			aliceKeyBytes, err := hex.DecodeString(aliceKeyStr)
			Expect(err).To(BeNil())
			aliceKey, err := crypto.ToECDSA(aliceKeyBytes)
			Expect(err).To(BeNil())
			aliceWallet, err := ethswap.NewWallet(aliceKey, client, swapAddr)
			Expect(err).To(BeNil())
			bobKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_2"), "0x")
			bobKeyBytes, err := hex.DecodeString(bobKeyStr)
			Expect(err).To(BeNil())
			bobKey, err := crypto.ToECDSA(bobKeyBytes)
			Expect(err).To(BeNil())
			bobWallet, err := ethswap.NewWallet(bobKey, client, swapAddr)
			Expect(err).To(BeNil())

			By("Get balance of both user")
			aliceBalance, err := aliceWallet.Balance(ctx, &tokenAddr, false)
			Expect(err).To(BeNil())
			bobBalance, err := bobWallet.Balance(ctx, &tokenAddr, false)
			Expect(err).To(BeNil())

			By("Alice constructs a swap")
			amount := big.NewInt(1e18)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			expiry := big.NewInt(6)
			swap, err := ethswap.NewSwap(aliceWallet.Address(), bobWallet.Address(), swapAddr, secretHash, amount, expiry)
			Expect(err).To(BeNil())

			By("Check status")
			initiated, err := swap.Initiated(ctx, client)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			redeemed, err := swap.Redeemed(ctx, client)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice initiates the swap")
			initTx, err := aliceWallet.Initiate(ctx, swap)
			Expect(err).To(BeNil())
			By(color.GreenString("Initiation tx hash = %v", initTx))
			time.Sleep(time.Second)

			By("Check status")
			initiated, err = swap.Initiated(ctx, client)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			redeemed, err = swap.Redeemed(ctx, client)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Bob redeems the swap")
			redeemTx, err := bobWallet.Redeem(ctx, swap, secret)
			Expect(err).To(BeNil())
			By(color.GreenString("Redeem tx hash = %v", redeemTx))
			time.Sleep(time.Second)
			redeemed, err = swap.Redeemed(ctx, client)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeTrue())
			// Expect(bytes.Equal(secret, revealedSecret))

			By("Check balance again")
			newAliceBalance, err := aliceWallet.Balance(ctx, &tokenAddr, false)
			Expect(err).To(BeNil())
			newBobBalance, err := bobWallet.Balance(ctx, &tokenAddr, false)
			Expect(err).To(BeNil())
			Expect(newAliceBalance.Cmp(big.NewInt(0).Sub(aliceBalance, amount))).Should(Equal(0))
			Expect(newBobBalance.Cmp(big.NewInt(0).Add(bobBalance, amount))).Should(Equal(0))
		})
	})

	Context("Alice wants to refund after expiry", func() {
		It("should work", func(ctx context.Context) {
			By("Initialise the client")
			url := os.Getenv("ETH_URL")
			client, err := ethclient.Dial(url)
			Expect(err).To(BeNil())

			By("Initialization two keys")
			aliceKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
			aliceKeyBytes, err := hex.DecodeString(aliceKeyStr)
			Expect(err).To(BeNil())
			aliceKey, err := crypto.ToECDSA(aliceKeyBytes)
			Expect(err).To(BeNil())
			aliceWallet, err := ethswap.NewWallet(aliceKey, client, swapAddr)
			Expect(err).To(BeNil())
			bobKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_2"), "0x")
			bobKeyBytes, err := hex.DecodeString(bobKeyStr)
			Expect(err).To(BeNil())
			bobKey, err := crypto.ToECDSA(bobKeyBytes)
			Expect(err).To(BeNil())
			bobWallet, err := ethswap.NewWallet(bobKey, client, swapAddr)
			Expect(err).To(BeNil())

			By("Get token balance")
			aliceBalance, err := aliceWallet.Balance(ctx, &tokenAddr, false)
			Expect(err).To(BeNil())

			By("Alice constructs a swap")
			amount := big.NewInt(1e18)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			expiry := big.NewInt(6)
			swap, err := ethswap.NewSwap(aliceWallet.Address(), bobWallet.Address(), swapAddr, secretHash, amount, expiry)
			Expect(err).To(BeNil())

			By("Alice initiates the swap")
			initTx, err := aliceWallet.Initiate(ctx, swap)
			Expect(err).To(BeNil())
			By(color.GreenString("Initiation tx hash = %v", initTx))
			time.Sleep(time.Second)

			By("Expect the balance to decrease")
			aliceBalance1, err := aliceWallet.Balance(ctx, &tokenAddr, false)
			Expect(err).To(BeNil())
			Expect(aliceBalance1.Cmp(big.NewInt(0).Sub(aliceBalance, amount))).Should(Equal(0))

			By("Wait for the swap to expire")
			time.Sleep(5 * time.Second)
			expired, err := swap.Expired(ctx, client)
			Expect(err).To(BeNil())
			Expect(expired).Should(BeTrue())

			By("Submit the refund tx")
			refundTx, err := aliceWallet.Refund(ctx, swap)
			Expect(err).To(BeNil())
			By(color.GreenString("refund tx hash = %v", refundTx))
			time.Sleep(time.Second)

			By("Expect the token balance to be same as the beginning")
			aliceBalance2, err := aliceWallet.Balance(ctx, &tokenAddr, false)
			Expect(err).To(BeNil())
			Expect(aliceBalance2.Cmp(aliceBalance)).Should(Equal(0))
		})
	})
})
