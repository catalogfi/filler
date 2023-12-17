package filler_test

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

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
	"github.com/catalogfi/cobi/pkg/cobid/filler"
	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
)

func generateOrder(
	id uint,
	initiatorInitAddr, initiatorRedeemAddr, followerInitAddr, followerRedeemAddr, maker, taker, orderPair string,
	initSwapStatus, followerSwapStatus model.SwapStatus,
	orderStatus model.Status,
	initTL, followerTL string,
	amount *big.Int,
	secret string, secretHash string) model.Order {

	initChain, followerChain, initAsset, followerAsset, err := model.ParseOrderPair(orderPair)
	if err != nil {
		log.Fatalf("%v", err)
	}

	order := model.Order{
		Maker:     maker,
		Taker:     taker,
		OrderPair: orderPair,
		InitiatorAtomicSwap: &model.AtomicSwap{
			Status:           initSwapStatus,
			SecretHash:       secretHash,
			Secret:           secret,
			InitiatorAddress: initiatorInitAddr,
			RedeemerAddress:  followerRedeemAddr,
			Timelock:         initTL,
			Chain:            initChain,
			Asset:            initAsset,
			Amount:           amount.String(),
		},
		FollowerAtomicSwap: &model.AtomicSwap{
			Status:           followerSwapStatus,
			SecretHash:       secretHash,
			Secret:           secret,
			InitiatorAddress: followerInitAddr,
			RedeemerAddress:  initiatorRedeemAddr,
			Timelock:         followerTL,
			Chain:            followerChain,
			Asset:            followerAsset,
			Amount:           amount.String(),
		},
		SecretHash: secretHash,
		Secret:     secret,
		Status:     orderStatus,
	}
	order.ID = id
	return order

}

var _ = Describe("Filler_setup", Ordered, func() {
	var fill filler.Filler
	var cobiEthWallet ethswap.Wallet
	var aliceEthWallet ethswap.Wallet
	var cobiBtcWallet btcswap.Wallet
	var aliceBtcWallet btcswap.Wallet
	var evmclient *ethclient.Client
	var fillstore *store.Store
	var btcclient btc.IndexerClient
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
		_, err = testutil.NigiriFaucet(aliceBtcWallet.Address().EncodeAddress())
		Expect(err).To(BeNil())

		err = testutil.NigiriNewBlock()
		Expect(err).To(BeNil())

		//eth wallet setup
		aliceKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
		aliceKeyBytes, err := hex.DecodeString(aliceKeyStr)
		Expect(err).To(BeNil())
		aliceKey, err := crypto.ToECDSA(aliceKeyBytes)
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

		aliceEthWallet, err = ethswap.NewWallet(aliceKey, evmclient, swapAddr)
		Expect(err).To(BeNil())

		logger, err := zap.NewDevelopment()
		Expect(err).To(BeNil())

		obWSClient := rest.NewWSClient(fmt.Sprintf("ws://%s/", orderBookUrl), logger.With(zap.String("wSclient", "orderbook")))
		obRestClient, err := cobiEthWallet.SIWEClient("http://localhost:8080")
		Expect(err).To(BeNil())

		quit := make(chan struct{})

		os.Remove("test.db")
		db, err := gorm.Open(sqlite.Open("test.db"))
		Expect(err).To(BeNil())

		store, err := store.NewStore(db)
		fillstore = &store
		Expect(err).To(BeNil())

		FillStrat := filler.StrategyWithDefaults(fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr))

		fill = filler.NewFiller(cobiBtcWallet, cobiEthWallet, obRestClient, obWSClient, *FillStrat, store, logger, quit)

		go func() {
			fill.Start()
		}()
	})
	AfterAll(func() {
		fill.Stop()
	})
	Context("Fill Orders According to Strategy", func() {

		BeforeAll(func() {

		})

		AfterAll(func() {

		})

		It("Fill Orders", func() {

		})

		It("Should not fill Orders with wrong strategy", func() {})

		It("Should fill multiples Strategies", func() {})

	})
})
