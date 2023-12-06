package ethswap_test

//
// import (
// 	"bytes"
// 	"context"
// 	"crypto/sha256"
// 	"encoding/hex"
// 	"math/big"
// 	"os"
// 	"strings"
// 	"time"
//
// 	"github.com/catalogfi/blockchain/testutil"
// 	"github.com/catalogfi/cobi/pkg/swap/ethswap"
// 	"github.com/catalogfi/cobi/pkg/swap/ethswap/bindings"
// 	"github.com/ethereum/go-ethereum/accounts/abi/bind"
// 	"github.com/ethereum/go-ethereum/crypto"
// 	"github.com/ethereum/go-ethereum/ethclient"
// 	"github.com/fatih/color"
// 	. "github.com/onsi/ginkgo/v2"
// 	. "github.com/onsi/gomega"
// )
//
// var _ = Describe("Ethereum Atomic Swap", func() {
// 	Context("Alice and Bob wants to do a swap", func() {
// 		It("should work", func(ctx context.Context) {
// 			By("Initialization two keys")
// 			aliceKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
// 			aliceKeyBytes, err := hex.DecodeString(aliceKeyStr)
// 			Expect(err).To(BeNil())
// 			aliceKey, err := crypto.ToECDSA(aliceKeyBytes)
// 			Expect(err).To(BeNil())
// 			aliceAddr := crypto.PubkeyToAddress(aliceKey.PublicKey)
// 			bobKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_2"), "0x")
// 			bobKeyBytes, err := hex.DecodeString(bobKeyStr)
// 			Expect(err).To(BeNil())
// 			bobKey, err := crypto.ToECDSA(bobKeyBytes)
// 			Expect(err).To(BeNil())
// 			bobAddr := crypto.PubkeyToAddress(bobKey.PublicKey)
//
// 			By("Initialise the client")
// 			url := os.Getenv("ETH_URL")
// 			client, err := ethclient.Dial(url)
// 			Expect(err).To(BeNil())
//
// 			By("Get balance of both user")
// 			tokenBinding, err := bindings.NewERC20(tokenAddr, client)
// 			Expect(err).To(BeNil())
// 			aliceBalance, err := tokenBinding.BalanceOf(&bind.CallOpts{}, aliceAddr)
// 			Expect(err).To(BeNil())
// 			bobBalance, err := tokenBinding.BalanceOf(&bind.CallOpts{}, bobAddr)
// 			Expect(err).To(BeNil())
//
// 			By("Alice constructs a swap")
// 			amount := big.NewInt(1e18)
// 			secret := testutil.RandomSecret()
// 			secretHash := sha256.Sum256(secret)
// 			expiry := big.NewInt(6)
// 			swap, err := ethswap.NewSwap(aliceAddr, bobAddr, swapAddr, client, secretHash, amount, expiry)
// 			Expect(err).To(BeNil())
//
// 			By("Check status")
// 			initiated, err := swap.Initiated()
// 			Expect(err).To(BeNil())
// 			Expect(initiated).Should(BeFalse())
// 			redeemed, _, err := swap.Redeemed(ctx, 0)
// 			Expect(err).To(BeNil())
// 			Expect(redeemed).Should(BeFalse())
//
// 			By("Alice initiates the swap")
// 			initTx, err := swap.Initiate(ctx, aliceKey)
// 			Expect(err).To(BeNil())
// 			By(color.GreenString("Initiation tx hash = %v", initTx))
// 			time.Sleep(time.Second)
// 			initiated, err = swap.Initiated()
// 			Expect(err).To(BeNil())
// 			Expect(initiated).Should(BeTrue())
// 			redeemed, _, err = swap.Redeemed(ctx, 0)
// 			Expect(err).To(BeNil())
// 			Expect(redeemed).Should(BeFalse())
//
// 			By("Bob redeems the swap")
// 			redeemTx, err := swap.Redeem(ctx, bobKey, secret)
// 			Expect(err).To(BeNil())
// 			By(color.GreenString("Redeem tx hash = %v", redeemTx))
// 			time.Sleep(time.Second)
// 			redeemed, revealedSecret, err := swap.Redeemed(ctx, 0)
// 			Expect(err).To(BeNil())
// 			Expect(redeemed).Should(BeTrue())
// 			Expect(bytes.Equal(secret, revealedSecret))
//
// 			By("Check balance again")
// 			newAliceBalance, err := tokenBinding.BalanceOf(&bind.CallOpts{}, aliceAddr)
// 			Expect(err).To(BeNil())
// 			newBobBalance, err := tokenBinding.BalanceOf(&bind.CallOpts{}, bobAddr)
// 			Expect(err).To(BeNil())
// 			Expect(newAliceBalance.Cmp(big.NewInt(0).Sub(aliceBalance, amount))).Should(Equal(0))
// 			Expect(newBobBalance.Cmp(big.NewInt(0).Add(bobBalance, amount))).Should(Equal(0))
// 		})
// 	})
//
// 	Context("Alice wants to refund after expiry", func() {
// 		It("should work", func(ctx context.Context) {
// 			By("Initialization two keys")
// 			aliceKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
// 			aliceKeyBytes, err := hex.DecodeString(aliceKeyStr)
// 			Expect(err).To(BeNil())
// 			aliceKey, err := crypto.ToECDSA(aliceKeyBytes)
// 			Expect(err).To(BeNil())
// 			aliceAddr := crypto.PubkeyToAddress(aliceKey.PublicKey)
// 			bobKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_2"), "0x")
// 			bobKeyBytes, err := hex.DecodeString(bobKeyStr)
// 			Expect(err).To(BeNil())
// 			bobKey, err := crypto.ToECDSA(bobKeyBytes)
// 			Expect(err).To(BeNil())
// 			bobAddr := crypto.PubkeyToAddress(bobKey.PublicKey)
//
// 			By("Initialise the client")
// 			url := os.Getenv("ETH_URL")
// 			client, err := ethclient.Dial(url)
// 			Expect(err).To(BeNil())
//
// 			By("Get token balance")
// 			tokenBinding, err := bindings.NewERC20(tokenAddr, client)
// 			Expect(err).To(BeNil())
// 			aliceBalance, err := tokenBinding.BalanceOf(&bind.CallOpts{}, aliceAddr)
// 			Expect(err).To(BeNil())
//
// 			By("Alice constructs a swap")
// 			amount := big.NewInt(1e18)
// 			secret := testutil.RandomSecret()
// 			secretHash := sha256.Sum256(secret)
// 			expiry := big.NewInt(3)
// 			swap, err := ethswap.NewSwap(aliceAddr, bobAddr, swapAddr, client, secretHash, amount, expiry)
// 			Expect(err).To(BeNil())
//
// 			By("Check status")
// 			initiated, err := swap.Initiated()
// 			Expect(err).To(BeNil())
// 			Expect(initiated).Should(BeFalse())
// 			redeemed, _, err := swap.Redeemed(ctx, 0)
// 			Expect(err).To(BeNil())
// 			Expect(redeemed).Should(BeFalse())
//
// 			By("Alice initiates the swap")
// 			initTx, err := swap.Initiate(ctx, aliceKey)
// 			Expect(err).To(BeNil())
// 			By(color.GreenString("Initiation tx hash = %v", initTx))
// 			time.Sleep(time.Second)
// 			initiated, err = swap.Initiated()
// 			Expect(err).To(BeNil())
// 			Expect(initiated).Should(BeTrue())
// 			redeemed, _, err = swap.Redeemed(ctx, 0)
// 			Expect(err).To(BeNil())
// 			Expect(redeemed).Should(BeFalse())
//
// 			By("Expect the balance to decrease")
// 			aliceBalance1, err := tokenBinding.BalanceOf(&bind.CallOpts{}, aliceAddr)
// 			Expect(err).To(BeNil())
// 			Expect(aliceBalance1.Cmp(big.NewInt(0).Sub(aliceBalance, amount))).Should(Equal(0))
//
// 			By("Wait for the swap to expire")
// 			time.Sleep(5 * time.Second)
// 			expired, err := swap.Expired(ctx)
// 			Expect(err).To(BeNil())
// 			Expect(expired).Should(BeTrue())
//
// 			By("Submit the refund tx")
// 			Expect(swap.Refund(ctx, aliceKey)).Should(Succeed())
// 			time.Sleep(time.Second)
//
// 			By("Expect the token balance to be same as the beginning")
// 			aliceBalance2, err := tokenBinding.BalanceOf(&bind.CallOpts{}, aliceAddr)
// 			Expect(err).To(BeNil())
// 			Expect(aliceBalance2.Cmp(aliceBalance)).Should(Equal(0))
// 		})
// 	})
// })
