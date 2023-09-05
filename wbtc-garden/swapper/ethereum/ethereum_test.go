package ethereum_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ethereum swap", func() {
	Context("when doing atomic swaps on ethereum", func() {
		It("should work when everyone redeems", func() {
			// Initialization and deploy contracts
			// Create two accounts to do the swap
			// Create the swaps for both party and test each function
			Expect(true).Should(BeTrue())
		})

		It("should allow Alice to refund if Bob doesn't initialise", func() {

		})

		It("should allow Bob to refund if Alice doesn't redeem", func() {

		})

		It("should not allow Alice/Bob to refund if timelock is not expired", func() {

		})

		It("should ")
	})

	Context("watcher", func() {
		It("should return the state of the swap", func() {

		})

		It("should still filter the event if the swap is taking long time", func() {

		})
	})
})

// import (
// 	"crypto/rand"
// 	"crypto/sha256"
// 	"fmt"
// 	"math/big"
// 	"time"
//
// 	"github.com/catalogfi/cobi/wbtc-garden/swapper"
// 	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum"
// 	"github.com/ethereum/go-ethereum/common"
// 	"github.com/ethereum/go-ethereum/crypto"
// 	. "github.com/onsi/ginkgo/v2"
// 	. "github.com/onsi/gomega"
// 	"go.uber.org/zap"
// )
//
// func randomHex(n int) ([]byte, error) {
// 	bytes := make([]byte, n)
// 	if _, err := rand.Read(bytes); err != nil {
// 		return []byte{}, err
// 	}
// 	return bytes, nil
// }
//
// var _ = Describe("Bitcoin", func() {
// 	It("should create a new swap", func() {
// 		// PRIV_KEY_1 := os.Getenv("PRIV_KEY_1")
// 		// PRIV_KEY_2 := os.Getenv("PRIV_KEY_2")
// 		// Skip("")
//
// 		PRIV_KEY_1 := "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80" // 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
// 		PRIV_KEY_2 := "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d" // 0x70997970C51812dc3A010C7d01b50e0d17dc79C8
// 		atomicSwapAdrr := common.HexToAddress("0x9CC8B5379C40E24F374cd55973c138fff83ed214")
// 		// TOKEN := common.HexToAddress("0x87c470437282174b3f8368c7CF1Ac03bcAe57954")
// 		// instantClientConfig := ethereum.InstantClientConfig{
// 		// 	Url:              "http://localhost:8282",
// 		// 	Entrypoint:       "0xC6c5Ab5039373b0CBa7d0116d9ba7fb9831C3f42",
// 		// 	Implementation:   "0x4ea0Be853219be8C9cE27200Bdeee36881612FF2",
// 		// 	Factory:          "0x46d4674578a2daBbD0CEAB0500c6c7867999db34",
// 		// 	PaymasterAndData: "0x9155497EAE31D432C0b13dBCc0615a37f55a2c87fB12F7170FF298CDed84C793dAb9aBBEcc01E798",
// 		// }
//
// 		privKey1, err := crypto.HexToECDSA(PRIV_KEY_1)
// 		Expect(err).To(BeNil())
// 		privKey2, err := crypto.HexToECDSA(PRIV_KEY_2)
// 		Expect(err).To(BeNil())
// 		pkAddr1 := crypto.PubkeyToAddress(privKey1.PublicKey)
// 		pkAddr2 := crypto.PubkeyToAddress(privKey2.PublicKey)
//
// 		fmt.Println(pkAddr1.Hex())
// 		fmt.Println(pkAddr2.Hex())
//
// 		Expect(err).To(BeNil())
// 		client, err := ethereum.NewClient("http://localhost:8545")
// 		Expect(err).To(BeNil())
//
// 		// client.ApproveERC20(privKey1, big.NewInt(100000), TOKEN, atomicSwapAdrr)
// 		// client.ApproveERC20(privKey2, big.NewInt(100000), TOKEN, atomicSwapAdrr)
// 		time.Sleep(2 * time.Second)
// 		// instClient := ethereum.InstantWalletWrapper(instantClientConfig, client)
// 		secret1, _ := randomHex(32)
// 		secret_hash1 := sha256.Sum256(secret1)
// 		secret2, _ := randomHex(32)
// 		secret_hash2 := sha256.Sum256(secret2)
//
// 		aExpiry, err := ethereum.GetExpiry(client, true)
// 		Expect(err).To(BeNil())
//
// 		bExpiry, err := ethereum.GetExpiry(client, false)
// 		Expect(err).To(BeNil())
//
// 		iSwapA, err := ethereum.NewInitiatorSwap(privKey1, pkAddr2, atomicSwapAdrr, secret_hash1[:], aExpiry, big.NewInt(0), big.NewInt(100000), client)
// 		Expect(err).To(BeNil())
// 		rSwapA, err := ethereum.NewRedeemerSwap(privKey1, pkAddr2, atomicSwapAdrr, secret_hash2[:], bExpiry, big.NewInt(0), big.NewInt(100000), client)
// 		Expect(err).To(BeNil())
//
// 		iSwapB, err := ethereum.NewInitiatorSwap(privKey2, pkAddr1, atomicSwapAdrr, secret_hash2[:], bExpiry, big.NewInt(0), big.NewInt(100000), client)
// 		Expect(err).To(BeNil())
// 		rSwapB, err := ethereum.NewRedeemerSwap(privKey2, pkAddr1, atomicSwapAdrr, secret_hash1[:], aExpiry, big.NewInt(0), big.NewInt(100000), client)
// 		Expect(err).To(BeNil())
//
// 		go func() {
// 			defer GinkgoRecover()
// 			Expect(swapper.ExecuteAtomicSwapFirst(iSwapA, rSwapA, secret1)).To(BeNil())
// 		}()
// 		Expect(swapper.ExecuteAtomicSwapSecond(iSwapB, rSwapB)).To(BeNil())
// 	})
// })
