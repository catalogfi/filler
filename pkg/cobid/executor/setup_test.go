package executor_test

// import (
// 	"encoding/hex"
// 	"fmt"
// 	"os"
// 	"strings"

// 	"github.com/btcsuite/btcd/chaincfg"
// 	"github.com/ethereum/go-ethereum/crypto"
// 	"github.com/ethereum/go-ethereum/ethclient"
// 	. "github.com/onsi/ginkgo/v2"
// 	. "github.com/onsi/gomega"
// 	"go.uber.org/zap"
// 	"gorm.io/driver/sqlite"
// 	"gorm.io/gorm"

// 	"github.com/catalogfi/blockchain/btc/btctest"
// 	"github.com/catalogfi/cobi/pkg/cobid/executor"
// 	"github.com/catalogfi/cobi/pkg/store"
// 	"github.com/catalogfi/cobi/pkg/swap/btcswap"
// 	"github.com/catalogfi/cobi/pkg/swap/ethswap"
// 	"github.com/catalogfi/orderbook/model"
// 	"github.com/catalogfi/orderbook/rest"
// )

// var _ = Describe("Executor_setup", func() {
// 	Context("Setup", func() {
// 		It("test setup", func() {
// 			Skip("Skip testing setup")
// 			orderBookUrl := "localhost:8080"
// 			network := &chaincfg.RegressionNetParams
// 			btcclient := btctest.RegtestIndexer()
// 			btcWallet, err := NewTestWallet(network, btcclient)
// 			Expect(err).To(BeNil())

// 			aliceKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
// 			aliceKeyBytes, err := hex.DecodeString(aliceKeyStr)
// 			Expect(err).To(BeNil())
// 			aliceKey, err := crypto.ToECDSA(aliceKeyBytes)
// 			Expect(err).To(BeNil())

// 			evmclient, err := ethclient.Dial(os.Getenv("ETH_URL"))
// 			Expect(err).To(BeNil())
// 			ethWallet, err := ethswap.NewWallet(aliceKey, evmclient, swapAddr)
// 			Expect(err).To(BeNil())

// 			btcChainMap := make(map[model.Chain]btcswap.Wallet)
// 			ethChainMap := make(map[model.Chain]ethswap.Wallet)

// 			btcChainMap[model.BitcoinRegtest] = btcWallet
// 			ethChainMap[model.EthereumLocalnet] = ethWallet

// 			logger, err := zap.NewDevelopment()
// 			Expect(err).To(BeNil())

// 			obclient := rest.NewWSClient(fmt.Sprintf("ws://%s/", orderBookUrl), logger.With(zap.String("client", "orderbook")))

// 			quit := make(chan struct{})

// 			db, err := gorm.Open(sqlite.Open("test.db"))
// 			Expect(err).To(BeNil())

// 			store, err := store.NewStore(db)
// 			Expect(err).To(BeNil())

// 			executor := executor.NewExecutor(btcWallet, ethWallet, ethWallet.Address(), obclient, executor.RegtestOptions(orderBookUrl), store, logger, quit)

// 			go func() {
// 				executor.Start()
// 			}()

// 			executor.Stop()
// 		})
// 	})
// })
