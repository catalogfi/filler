package swapper_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/cobi/wbtc-garden/swapper"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

func randomHex(n int) ([]byte, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return []byte{}, err
	}
	return bytes, nil
}

var _ = Describe("Ethereum to Bitcoin", func() {
	It("should create a new swap", func() {
		// TO:DO Setup Automation
		// BTC
		// nigiri stop --delete
		// nigiri faucet mvb8yA23gtNPsBpd21Wq5J6YY4GEnfYQyX
		// nigiri faucet myS2zesC4Va7ofV5MtnqZDct8iZdaBzULE
		// nigiri start

		// ETH
		// npx hardhat node

		// PRIV_KEY_1 := os.Getenv("PRIV_KEY_1")
		// PRIV_KEY_2 := os.Getenv("PRIV_KEY_2")
		// Skip("")

		PRIV_KEY_1 := "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
		//eth:0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266 btc:mvb8yA23gtNPsBpd21Wq5J6YY4GEnfYQyX
		PRIV_KEY_2 := "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
		//eth:0x70997970C51812dc3A010C7d01b50e0d17dc79C8 btc:myS2zesC4Va7ofV5MtnqZDct8iZdaBzULE
		ETH_ATOMICSWAP := common.HexToAddress("0x9CC8B5379C40E24F374cd55973c138fff83ed214")
		TOKEN := common.HexToAddress("0x87c470437282174b3f8368c7CF1Ac03bcAe57954")
		// ETH_ATOMICSWAP, TOKEN := Setup(PRIV_KEY_1, PRIV_KEY_2) //fails with Method eth_maxPriorityFeePerGas not found (hardhat issue)

		btcPrivKeyBytes1, _ := hex.DecodeString(PRIV_KEY_1)
		btcPrivKey1, _ := btcec.PrivKeyFromBytes(btcPrivKeyBytes1)

		btcPrivKeyBytes2, _ := hex.DecodeString(PRIV_KEY_2)
		btcPrivKey2, _ := btcec.PrivKeyFromBytes(btcPrivKeyBytes2)

		btcPkAddr1, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(btcPrivKey1.PubKey().SerializeCompressed()), &chaincfg.RegressionNetParams)
		Expect(err).To(BeNil())
		fmt.Println("btcPkAddr1:", btcPkAddr1.EncodeAddress())

		btcPkAddr2, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(btcPrivKey2.PubKey().SerializeCompressed()), &chaincfg.RegressionNetParams)
		Expect(err).To(BeNil())
		fmt.Println("btcPkAddr2:", btcPkAddr2.EncodeAddress())

		ethPrivKey1, err := crypto.HexToECDSA(PRIV_KEY_1)
		Expect(err).To(BeNil())
		ethPrivKey2, err := crypto.HexToECDSA(PRIV_KEY_2)
		Expect(err).To(BeNil())
		ethPkAddr1 := crypto.PubkeyToAddress(ethPrivKey1.PublicKey)
		ethPkAddr2 := crypto.PubkeyToAddress(ethPrivKey2.PublicKey)

		fmt.Println("ethPkAddr1", ethPkAddr1.Hex())
		fmt.Println("ethPkAddr2", ethPkAddr2.Hex())

		logger, err := zap.NewDevelopment()
		Expect(err).To(BeNil())
		ethClient, err := ethereum.NewClient(logger, "http://localhost:8545")
		Expect(err).To(BeNil())

		btcClient := bitcoin.NewClient("https://mempool.space/testnet/api", &chaincfg.RegressionNetParams)

		ethClient.ApproveERC20(ethPrivKey1, big.NewInt(100000), TOKEN, ETH_ATOMICSWAP)

		secret, _ := randomHex(32)
		secret_hash := sha256.Sum256(secret)

		btcExpiry := int64(1000)
		ethExpiry := big.NewInt(100000)

		erc20BalanceOfPK1, _ := ethClient.GetERC20Balance(TOKEN, ethPkAddr1)
		erc20BalanceOfPK2, _ := ethClient.GetERC20Balance(TOKEN, ethPkAddr2)

		_, btcBalanceOfPK1, _ := btcClient.GetUTXOs(btcPkAddr1, 0)
		_, btcBalanceOfPK2, _ := btcClient.GetUTXOs(btcPkAddr2, 0)

		iSwapA, err := ethereum.NewInitiatorSwap(ethPrivKey1, ethPkAddr2, ETH_ATOMICSWAP, secret_hash[:], ethExpiry, big.NewInt(0), big.NewInt(100000), ethClient, 10000)
		Expect(err).To(BeNil())
		rSwapA, err := bitcoin.NewRedeemerSwap(logger, btcPrivKey1, btcPkAddr2, secret_hash[:], btcExpiry, 0, 10000, btcClient)
		Expect(err).To(BeNil())

		iSwapB, err := bitcoin.NewInitiatorSwap(logger, btcPrivKey2, btcPkAddr1, secret_hash[:], btcExpiry, 0, 10000, btcClient)
		Expect(err).To(BeNil())
		rSwapB, err := ethereum.NewRedeemerSwap(ethPrivKey2, ethPkAddr1, ETH_ATOMICSWAP, secret_hash[:], ethExpiry, big.NewInt(0), big.NewInt(100000), ethClient, 10000)
		Expect(err).To(BeNil())

		go func() {
			defer GinkgoRecover()
			Expect(swapper.ExecuteAtomicSwapFirst(iSwapA, rSwapA, secret)).To(BeNil())
		}()
		Expect(swapper.ExecuteAtomicSwapSecond(iSwapB, rSwapB)).To(BeNil())

		erc20BalanceOfPK1After, _ := ethClient.GetERC20Balance(TOKEN, ethPkAddr1)
		erc20BalanceOfPK2After, _ := ethClient.GetERC20Balance(TOKEN, ethPkAddr2)

		_, btcBalanceOfPK1After, _ := btcClient.GetUTXOs(btcPkAddr1, 0)
		_, btcBalanceOfPK2After, _ := btcClient.GetUTXOs(btcPkAddr2, 0)

		time.Sleep(10 * time.Second)

		fmt.Println("ERC20 balance of PK1:", erc20BalanceOfPK1, erc20BalanceOfPK1After)
		fmt.Println("ERC20 balance of PK2:", erc20BalanceOfPK2, erc20BalanceOfPK2After)
		fmt.Println("BTC balance of PK1  :", btcBalanceOfPK1, btcBalanceOfPK1After)
		fmt.Println("BTC balance of PK2  :", btcBalanceOfPK2, btcBalanceOfPK2After)
	})
})
