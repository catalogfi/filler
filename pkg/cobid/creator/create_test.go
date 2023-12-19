package creator_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
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
	"github.com/catalogfi/orderbook/model"
)

func generaterderWithDefaults(
	orderPair string,
	orderStatus model.Status,
	sendAmount, receiveAmount *big.Int,
	maker, taker string,
) model.Order {

	price, _ := new(big.Float).Quo(new(big.Float).SetInt(sendAmount), new(big.Float).SetInt(receiveAmount)).Float64()
	secret := testutil.RandomSecret()
	secretHash := sha256.Sum256(secret)

	if maker == "" {
		maker = "0xa31Fe4c53BFe658A4B98EF81B88F4F1bAffE62f8"
	}
	if taker == "" {
		taker = "0x09FA41e14B53166c368CED4E743489184F3458ac"
	}

	order := model.Order{
		Maker: maker,
		Taker: taker,
		Price: price,

		OrderPair:           orderPair,
		InitiatorAtomicSwap: &model.AtomicSwap{},
		FollowerAtomicSwap: &model.AtomicSwap{
			Amount: receiveAmount.String(),
		},
		SecretHash: hex.EncodeToString(secretHash[:]),
		Secret:     hex.EncodeToString(secret[:]),
		Status:     orderStatus,
	}
	order.ID = uint(rand.Intn(100000))
	return order

}

func GetStore() store.Store {
	db, err := gorm.Open(sqlite.Open("test.db"))
	Expect(err).To(BeNil())
	checkUpStore, err := store.NewStore(db)
	Expect(err).To(BeNil())
	return checkUpStore
}

var _ = Describe("Creator_setup", Ordered, func() {
	var create creator.Creator
	var cobiEthWallet ethswap.Wallet
	var cobiBtcWallet btcswap.Wallet
	var aliceBtcWallet btcswap.Wallet
	var evmclient *ethclient.Client
	var btcclient btc.IndexerClient
	var clossureFunc func(CreateStrat *creator.Strategy) creator.Creator
	BeforeAll(func() {

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

		obRestClient, err := cobiEthWallet.SIWEClient("http://localhost:8080")
		Expect(err).To(BeNil())

		quit := make(chan struct{})

		os.Remove("test.db")
		db, err := gorm.Open(sqlite.Open("test.db"))
		Expect(err).To(BeNil())

		store, err := store.NewStore(db)
		Expect(err).To(BeNil())

		// CreateStrat := creator.StrategyWithDefaults(fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr))
		CreateStrat := creator.NewStrategy(
			6, 12, new(big.Int).SetInt64(1e6), fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr), 10,
		)

		clossureFunc = func(CreateStrat *creator.Strategy) creator.Creator {
			return creator.NewCreator(cobiBtcWallet, cobiEthWallet, obRestClient, *CreateStrat, store, logger, quit)
		}

		create = clossureFunc(CreateStrat)

		go func() {
			create.Start()
		}()
	})
	AfterAll(func() {
		create.Stop()
	})
	Context("Create Orders According to Strategy", func() {
		It("should have created Orders", func() {
			// sleep for one minute atleast 5 orders should have been created atmost 10
			time.Sleep(60 * time.Second)
			_, err := GetStore().OrderByID(5)
			Expect(err).To(BeNil())

			_, err = GetStore().OrderByID(10)
			Expect(err).ToNot(BeNil())
		})
	})
})
