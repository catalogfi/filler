package ethswap_test

import (
	"bytes"
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
			options := ethswap.OptionsLocalnet(swapAddr)
			aliceKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
			aliceKeyBytes, err := hex.DecodeString(aliceKeyStr)
			Expect(err).To(BeNil())
			aliceKey, err := crypto.ToECDSA(aliceKeyBytes)
			Expect(err).To(BeNil())

			aliceWallet, err := ethswap.NewWallet(options, aliceKey, client)
			Expect(err).To(BeNil())
			bobKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_2"), "0x")
			bobKeyBytes, err := hex.DecodeString(bobKeyStr)
			Expect(err).To(BeNil())
			bobKey, err := crypto.ToECDSA(bobKeyBytes)
			Expect(err).To(BeNil())
			bobWallet, err := ethswap.NewWallet(options, bobKey, client)
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
			swap := ethswap.NewSwap(aliceWallet.Address(), bobWallet.Address(), swapAddr, secretHash, amount, expiry)

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
			revealedSecret, err := swap.Secret(ctx, client, 500)
			Expect(err).To(BeNil())
			Expect(bytes.Equal(secret, revealedSecret))

			By("Check balance again")
			newAliceBalance, err := aliceWallet.Balance(ctx, &tokenAddr, false)
			Expect(err).To(BeNil())
			newBobBalance, err := bobWallet.Balance(ctx, &tokenAddr, false)
			Expect(err).To(BeNil())
			Expect(newAliceBalance.Cmp(big.NewInt(0).Sub(aliceBalance, amount))).Should(Equal(0))
			Expect(newBobBalance.Cmp(big.NewInt(0).Add(bobBalance, amount))).Should(Equal(0))
		})
	})
})
