package filler_test

import (
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	testutil2 "github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/pkg/cobid/filler"
	"github.com/catalogfi/cobi/pkg/mock"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"github.com/ethereum/go-ethereum/trie/testutil"
	"go.uber.org/zap"
	"gorm.io/gorm"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filler", func() {
	Context("when initializing the Filler", func() {
		It("should check the addresses of each order pair", func() {
			By("Create strategies")
			orderPair := fmt.Sprintf("ethereum:%s-bitcoin", testutil.RandomAddress().Hex())
			from, err := testutil2.RandomBtcAddressP2PKH(&chaincfg.MainNetParams)
			Expect(err).Should(BeNil())
			to := testutil.RandomAddress()
			stg, err := filler.NewStrategy(orderPair, from.EncodeAddress(), to.Hex(), nil, big.NewInt(1e8), big.NewInt(1e9), 10)
			Expect(err).Should(BeNil())
			stgs := []filler.Strategy{stg}

			By("Create orderbook clients")
			logger, err := zap.NewDevelopment()
			Expect(err).Should(BeNil())
			restClient := mock.NewOrderbookClient()
			wsClient := mock.NewOrderbookWsClient()

			By("Simulate an order from the orderbook")
			order := model.Order{
				Model: gorm.Model{
					ID: uint(rand.Uint64()),
				},
				Maker: "abc",
				FollowerAtomicSwap: &model.AtomicSwap{
					Amount: "200000000",
				},
				Price: 1.5,
			}
			orderQueue := make(chan interface{}, 1)
			orderQueue <- rest.OpenOrders{
				Orders: []model.Order{order},
				Error:  "",
			}
			wsClient.FuncListen = func() <-chan interface{} {
				return orderQueue
			}
			orderFilled := false
			restClient.FuncFillOrder = func(id uint, from string, to string) error {
				if id != order.ID || from != stg.SendAddress || to != stg.ReceiveAddress {
					return fmt.Errorf("wrong")
				}
				orderFilled = true
				return nil
			}

			By("Create the filler")
			filler := filler.New(stgs, restClient, wsClient, logger)

			By("Start the filler")
			Expect(filler.Start()).Should(Succeed())
			defer filler.Stop()

			By("Filler should fill the order")
			time.Sleep(time.Second)
			Expect(orderFilled).Should(BeTrue())
		})
	})
})
