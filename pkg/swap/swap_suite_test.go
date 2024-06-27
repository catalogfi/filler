package swap_test

// import (
// 	"context"
// 	"crypto/ecdsa"
// 	"encoding/hex"
// 	"os"
// 	"strings"
// 	"testing"

// 	"github.com/btcsuite/btcd/btcec/v2"
// 	"github.com/catalogfi/blockchain/btc"
// 	"github.com/ethereum/go-ethereum/accounts/abi/bind"
// 	"github.com/ethereum/go-ethereum/common"
// 	"github.com/ethereum/go-ethereum/core/types"
// 	"github.com/ethereum/go-ethereum/crypto"
// 	"github.com/ethereum/go-ethereum/ethclient"
// 	"github.com/fatih/color"
// 	"go.uber.org/zap"

// 	. "github.com/onsi/ginkgo/v2"
// 	. "github.com/onsi/gomega"
// )

// var (
// 	swapAddr  common.Address
// 	tokenAddr common.Address

// 	indexer btc.IndexerClient
// )

// var _ = BeforeSuite(func() {
// 	By("Required envs")
// 	Expect(os.Getenv("ETH_URL")).ShouldNot(BeEmpty())
// 	Expect(os.Getenv("ETH_KEY_1")).ShouldNot(BeEmpty())
// 	Expect(os.Getenv("BTC_INDEXER_ELECTRS_REGNET")).ShouldNot(BeEmpty())

// 	By("Initialise client")
// 	url := os.Getenv("ETH_URL")
// 	client, err := ethclient.Dial(url)
// 	Expect(err).Should(BeNil())
// 	chainID, err := client.ChainID(context.Background())
// 	Expect(err).Should(BeNil())

// 	By("Initialise transactor")
// 	keyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
// 	keyBytes, err := hex.DecodeString(keyStr)
// 	Expect(err).Should(BeNil())
// 	key, err := crypto.ToECDSA(keyBytes)
// 	Expect(err).Should(BeNil())
// 	transactor, err := bind.NewKeyedTransactorWithChainID(key, chainID)
// 	Expect(err).Should(BeNil())

// 	By("Deploy ERC20 contract")
// 	var tx *types.Transaction
// 	tokenAddr, tx, _, err = bindings.DeployTestERC20(transactor, client)
// 	Expect(err).Should(BeNil())
// 	_, err = bind.WaitMined(context.Background(), client, tx)
// 	Expect(err).Should(BeNil())
// 	By(color.GreenString("ERC20 deployed to %v", tokenAddr.Hex()))

// 	By("Deploy atomic swap contract")
// 	swapAddr, tx, _, err = bindings.DeployGardenHTLC(transactor, client, tokenAddr, "Garden", "v0")
// 	Expect(err).Should(BeNil())
// 	_, err = bind.WaitMined(context.Background(), client, tx)
// 	Expect(err).Should(BeNil())
// 	By(color.GreenString("Atomic swap deployed to %v", swapAddr.Hex()))

// 	By("Bitcoin envs")
// 	logger, err := zap.NewDevelopment()
// 	Expect(err).Should(BeNil())
// 	indexerHost, ok := os.LookupEnv("BTC_REGNET_INDEXER")
// 	Expect(ok).Should(BeTrue())
// 	indexer = btc.NewElectrsIndexerClient(logger, indexerHost, btc.DefaultRetryInterval)
// })

// func TestSwapper(t *testing.T) {
// 	RegisterFailHandler(Fail)
// 	RunSpecs(t, "Swapper Suite")
// }

// func ParseKeyFromEnv(name string) (*btcec.PrivateKey, *ecdsa.PrivateKey, error) {
// 	keyStr := strings.TrimPrefix(os.Getenv(name), "0x")
// 	keyBytes, err := hex.DecodeString(keyStr)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	ecdsaKey, err := crypto.ToECDSA(keyBytes)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	btcKey, _ := btcec.PrivKeyFromBytes(keyBytes)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	return btcKey, ecdsaKey, nil
// }
