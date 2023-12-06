package executor_test

import (
	"encoding/hex"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/catalogfi/blockchain/btc/btctest"
	"github.com/catalogfi/cobi/pkg/cobid/executor"
	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
)

var _ = Describe("Executor", func() {
	Context("Should be able to excute a trade from wbtc to btc", func() {
		orderBookUrl := ""
		network := &chaincfg.RegressionNetParams
		btcclient := btctest.RegtestIndexer()
		btcWallet, err := NewTestWallet(network, btcclient)
		Expect(err).To(BeNil())

		cobiKeyBytes, err := hex.DecodeString("")
		Expect(err).To(BeNil())
		cobiKey, err := crypto.ToECDSA(cobiKeyBytes)
		Expect(err).To(BeNil())
		cobiAddr := crypto.PubkeyToAddress(cobiKey.PublicKey)
		evmclient, err := ethclient.Dial("")
		Expect(err).To(BeNil())
		ethWallet, err := ethswap.NewWallet(cobiKey, evmclient, common.HexToAddress("0x"))
		Expect(err).To(BeNil())

		btcChainMap := make(map[model.Chain]btcswap.Wallet)
		ethChainMap := make(map[model.Chain]ethswap.Wallet)

		btcChainMap[model.BitcoinRegtest] = btcWallet
		ethChainMap[model.EthereumLocalnet] = ethWallet

		logger, err := zap.NewDevelopment()
		Expect(err).To(BeNil())

		quit := make(chan struct{})

		db, err := gorm.Open(sqlite.Open("test.db"))
		Expect(err).To(BeNil())

		store, err := store.NewStore(db)
		Expect(err).To(BeNil())

		executor := executor.NewExecutor(orderBookUrl, store, logger, quit)

		go executor.Start(btcChainMap, ethChainMap, cobiAddr.String())

	})

})
