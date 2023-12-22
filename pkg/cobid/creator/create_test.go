package creator_test

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/blockchain/btc/btctest"
	"github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/pkg/cobid/creator"
	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/rest"
)

var _ = Describe("Creator_setup", Ordered, func() {
	var create creator.Creator
	var cobiEthWallet ethswap.Wallet
	var cobiBtcWallet btcswap.Wallet
	var aliceBtcWallet btcswap.Wallet
	var evmclient *ethclient.Client
	var btcclient btc.IndexerClient
	var clossureFunc func(CreateStrat *creator.Strategy) creator.Creator
	var createStore store.Store
	BeforeAll(func() {
		orderBookUrl := "localhost:8080"

		var err error

		//btc wallet setup
		network := &chaincfg.RegressionNetParams
		btcclient = btctest.RegtestIndexer()
		cobiBtcWallet, err = NewTestWallet(network, btcclient)
		Expect(err).To(BeNil())
		_, err = testutil.NigiriFaucet(cobiBtcWallet.Address().EncodeAddress())
		Expect(err).To(BeNil())

		aliceBtcWallet, err = NewTestWallet(network, btcclient)
		Expect(err).To(BeNil())
		_, err = testutil.NigiriFaucet(aliceBtcWallet.Address().EncodeAddress()) // fund and mine
		Expect(err).To(BeNil())

		cobiKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_2"), "0x")
		cobiKeyBytes, err := hex.DecodeString(cobiKeyStr)
		Expect(err).To(BeNil())
		cobiKey, err := crypto.ToECDSA(cobiKeyBytes)
		Expect(err).To(BeNil())

		evmclient, err = ethclient.Dial(os.Getenv("ETH_URL"))
		Expect(err).To(BeNil())

		cobiEthWallet, err = ethswap.NewWallet(cobiKey, evmclient, swapAddr)
		Expect(err).To(BeNil())

		logger, err := zap.NewDevelopment()
		Expect(err).To(BeNil())

		obRestClient := rest.NewClient("http://"+orderBookUrl, cobiKeyStr)
		jwt, err := obRestClient.Login()
		Expect(err).To(BeNil())
		err = obRestClient.SetJwt(jwt)
		Expect(err).To(BeNil())

		os.Remove("test.db")
		db, err := gorm.Open(sqlite.Open("test.db"))
		Expect(err).To(BeNil())

		createStore, err = store.NewStore(db)
		Expect(err).To(BeNil())

		// CreateStrat := creator.StrategyWithDefaults(fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr))
		CreateStrat := creator.NewStrategy(
			6, 12, new(big.Int).SetInt64(1e6), fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr), 10,
		)

		clossureFunc = func(CreateStrat *creator.Strategy) creator.Creator {
			// fmt.Println("addresses" + cobiBtcWallet.Address().EncodeAddress() + " " + cobiEthWallet.Address().String())
			creator, err := creator.NewCreator(cobiBtcWallet.Address().EncodeAddress(), cobiEthWallet.Address().String(), obRestClient, *CreateStrat, createStore, logger)
			Expect(err).To(BeNil())
			return creator
		}

		create = clossureFunc(CreateStrat)

		err = create.Start()
		Expect(err).To(BeNil())

	})

	AfterAll(func() {
		create.Stop()
	})

	Context("Create Orders According to Strategy", func() {
		It("should have created Orders", func() {
			// sleep for one minute atleast 5 orders should have been created atmost 10
			time.Sleep(60 * time.Second)
			_, err := createStore.OrderByID(5) // read Operation on db
			Expect(err).To(BeNil())

			_, err = createStore.OrderByID(10)
			Expect(err).ToNot(BeNil())
		})
	})
})
