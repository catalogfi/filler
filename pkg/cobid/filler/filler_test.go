package filler_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/pkg/cobid/filler"
	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
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

var _ = Describe("Filler_setup", Ordered, func() {
	var fill filler.Filler
	var fillstore store.Store
	var clossureFunc func(FillStrat *filler.Strategy) filler.Filler
	var tokenAddr common.Address
	BeforeAll(func() {
		orderBookUrl := "localhost:8080"

		var err error

		logger, err := zap.NewDevelopment()
		Expect(err).To(BeNil())

		// prepare clients
		cobiKeyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_2"), "0x")
		obWSClient := rest.NewWSClient(fmt.Sprintf("ws://%s/", orderBookUrl), logger.With(zap.String("wSclient", "orderbook")))
		obRestClient := rest.NewClient("http://"+orderBookUrl, cobiKeyStr)
		jwt, err := obRestClient.Login()
		Expect(err).To(BeNil())
		err = obRestClient.SetJwt(jwt)
		Expect(err).To(BeNil())

		os.Remove("test.db")
		db, err := gorm.Open(sqlite.Open("test.db"))
		Expect(err).To(BeNil())

		fillstore, err = store.NewStore(db)
		Expect(err).To(BeNil())

		swapKey, err := crypto.GenerateKey()
		Expect(err).To(BeNil())

		tokenAddr = crypto.PubkeyToAddress(swapKey.PublicKey)

		// FillStrat := filler.StrategyWithDefaults(fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr))
		FillStrat := filler.NewStrategy(
			[]string{"0xa31Fe4c53BFe658A4B98EF81B88F4F1bAffE62f8"}, big.NewInt(1e6), big.NewInt(1e8),
			fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr), float64(10000)/float64(10000-100),
		)

		ethKey, err := crypto.GenerateKey()
		Expect(err).To(BeNil())
		btcKey, err := btcec.NewPrivateKey()
		Expect(err).To(BeNil())
		addr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(btcKey.PubKey().SerializeCompressed()), &chaincfg.RegressionNetParams)
		Expect(err).To(BeNil())

		clossureFunc = func(FillStrat *filler.Strategy) filler.Filler {
			filler, err := filler.NewFiller(addr.EncodeAddress(), crypto.PubkeyToAddress(ethKey.PublicKey).Hex(), obRestClient, obWSClient, *FillStrat, fillstore, logger)
			Expect(err).To(BeNil())
			return filler
		}

		fill = clossureFunc(FillStrat)

		err = fill.Start()
		Expect(err).To(BeNil())
	})

	AfterAll(func() {
		fill.Stop()
	})

	Context("Fill Orders According to Strategy", func() {

		var amount *big.Int

		BeforeAll(func() {
			amount = big.NewInt(1e7)

		})

		It("Fill Orders", func() {
			order := generaterderWithDefaults(
				fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr),
				model.Created, amount, new(big.Int).Sub(amount, big.NewInt(1e5)),
				"", "",
			)
			server.Msg <- rest.OpenOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			time.Sleep(1 * time.Second)
			Storeorder, err := fillstore.OrderBySecretHash(order.SecretHash) // read Operation on db
			Expect(err).To(BeNil())
			Expect(uint(Storeorder.OrderId)).To(Equal(order.ID))
		})

		It("Should not fill Orders with wrong strategy", func() {

			// less price
			order := generaterderWithDefaults(
				fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr),
				model.Created, amount, new(big.Int).Sub(amount, big.NewInt(0)),
				"", "",
			)
			server.Msg <- rest.OpenOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			_, err := fillstore.OrderBySecretHash(order.SecretHash)
			Expect(err).ToNot(BeNil())

			// invalid maker
			order = generaterderWithDefaults(
				fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr),
				model.Created, amount, new(big.Int).Sub(amount, big.NewInt(1e5)),
				"0x09FA41e14B53166c368CED4E763489184F3458ad", "",
			)

			server.Msg <- rest.OpenOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			_, err = fillstore.OrderBySecretHash(order.SecretHash)
			Expect(err).ToNot(BeNil())

			// amount less than minAmount in strategy
			amount = big.NewInt(1e5)
			order = generaterderWithDefaults(
				fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr),
				model.Created, amount, new(big.Int).Sub(amount, big.NewInt(1e3)),
				"0x09FA41e14B53166c368CED4E763489184F3458ad", "",
			)

			server.Msg <- rest.OpenOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			_, err = fillstore.OrderBySecretHash(order.SecretHash)
			Expect(err).ToNot(BeNil())

			// amount greater than minAmount in strategy
			amount = big.NewInt(1e15)
			order = generaterderWithDefaults(
				fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr),
				model.Created, amount, new(big.Int).Sub(amount, big.NewInt(1e14)),
				"0x09FA41e14B53166c368CED4E763489184F3458ad", "",
			)

			server.Msg <- rest.OpenOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			_, err = fillstore.OrderBySecretHash(order.SecretHash)
			Expect(err).ToNot(BeNil())

		})

		// It("Should not fill Orders with wrong strategy", func() {
		// 	// fill.Stop()
		// 	FillStrat := filler.NewStrategy(
		// 		[]string{"0xa31Fe4c53BFe658A4B98EF81B88F4F1bAffE62f8"}, big.NewInt(1e6), big.NewInt(1e8),
		// 		fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr), float64(10000)/float64(10000-1000),
		// 	)

		// 	fill = clossureFunc(FillStrat)
		// 	go func() {
		// 		fill.Start()
		// 	}()

		// 	order := generaterderWithDefaults(
		// 		fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr),
		// 		model.Created, amount, new(big.Int).Sub(amount, big.NewInt(1e7)),
		// 	)
		// 	server.Msg <- rest.OpenOrders{
		// 		Orders: []model.Order{order},
		// 		Error:  "",
		// 	}

		// 	_, err := fillstore.OrderBySecretHash(order.SecretHash)
		// 	Expect(err).ToNot(BeNil())

		// 	fill.Stop()
		// })

		// It("Should fill multiples Strategies", func() {
		// 	// define multiple strategies
		// 	Strategy := []filler.Strategy{
		// 		// valid maker
		// 		*filler.NewStrategy(
		// 			[]string{"0xa31Fe4c53BFe658A4B98EF81B88F4F1bAffE62f8"}, big.NewInt(1e6), big.NewInt(1e8),
		// 			fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr), float64(10000)/float64(10000-100),
		// 		),
		// 	}

		// })

	})
})
