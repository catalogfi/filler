package store_test

import (
	"os"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	. "github.com/catalogfi/cobi/wbtc-garden/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// var config = map[model.Chain]string{
// 	model.Bitcoin:  "http://127.0.0.1:30000",
// 	model.Ethereum: "http://127.0.0.1:8545",
// }

var config = model.Config{}

var _ = Describe("Store", func() {
	It("should be able to get locked amount", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:ETH-BTC:BTC", "100", "200", "secretHash", "receivebtcAddress", config)

		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("creator", "sendAddress", "receiveAddress", "bitcoin-ethereum", "100", "200", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())
		initiatorUnfilledOrders, err := store.FilterOrders("creator", "", "ethereum-bitcoin", "", "", model.OrderCreated, 0, 0, 0, 0, 0, 0, true)
		Expect(err).NotTo(HaveOccurred())
		order := initiatorUnfilledOrders[0]
		order.Status = model.InitiatorAtomicSwapInitiated
		store.UpdateOrder(&order)
		followerUnfilledOrders, err := store.FilterOrders("creator", "", "bitcoin-ethereum", "", "", model.Status(1), 0, 0, 0, 0, 0, 0, true)
		Expect(err).NotTo(HaveOccurred())
		order = followerUnfilledOrders[0]
		order.Status = model.InitiatorAtomicSwapRedeemed
		store.UpdateOrder(&order)
		amount, err := store.GetValueLocked(config, model.Ethereum)
		Expect(err).NotTo(HaveOccurred())
		Expect(amount).To(Equal(int64(300)))
		// Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})
	// It("should be able to create an order", func() {
	// 	store, err := New(sqlite.Open("test.db"), &gorm.Config{})
	// 	Expect(err).NotTo(HaveOccurred())
	// 	id, err := store.CreateOrder("creator", "sendAddress", "receiveAddress", "ethereum:primary-bitcoin:primary", "100", "200", "secretHash", config)
	// 	Expect(err).NotTo(HaveOccurred())
	// 	order, err := store.GetOrder(id)
	// 	Expect(err).NotTo(HaveOccurred())
	// 	Expect(order.ID).To(Equal(id))
	// 	Expect(order.Maker).To(Equal("creator"))
	// 	Expect(order.InitiatorAtomicSwap.InitiatorAddress).To(Equal("sendAddress"))
	// 	Expect(order.FollowerAtomicSwap.RedeemerAddress).To(Equal("receiveAddress"))
	// 	Expect(order.InitiatorAtomicSwap.Amount).To(Equal("100"))
	// 	Expect(order.FollowerAtomicSwap.Amount).To(Equal("200"))
	// 	Expect(order.SecretHash).To(Equal("secretHash"))
	// 	Expect(order.OrderPair).To(Equal("ethereum:primary-bitcoin:primary"))
	// 	Expect(order.Price).To(Equal(float64(0.5)))
	// 	Expect(order.Status).To(Equal(model.OrderCreated))
	// 	Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	// })

	It("should be able to fill an order", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:ETH-BTC:BTC", "100", "200", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(id, "filler", "sendFollowerAddress", "reciveFollowerAddress", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(id, "filler2", "sendFollowerAddress2", "reciveFollowerAddress2", config)
		Expect(err).To(HaveOccurred())
		order, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())
		Expect(order.Taker).To(Equal("filler"))
		Expect(order.InitiatorAtomicSwap.RedeemerAddress).To(Equal("reciveFollowerAddress"))
		Expect(order.FollowerAtomicSwap.InitiatorAddress).To(Equal("sendFollowerAddress"))
		Expect(order.InitiatorAtomicSwap.Timelock).To(Equal(uint64(144)))
		Expect(order.FollowerAtomicSwap.Timelock).To(Equal(uint64(144)))
		Expect(order.Status).To(Equal(model.OrderFilled))
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("should be able to cancel an order", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		cid, err := store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:ETH-BTC:BTC", "100", "200", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())
		order, err := store.GetOrder(cid)
		Expect(err).NotTo(HaveOccurred())
		Expect(order.ID).To(Equal(cid))
		err = store.CancelOrder("creator", cid)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.GetOrder(cid)
		Expect(err).To(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("shouldn't be able to cancel a filled order", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		cid, err := store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:ETH-BTC:BTC", "100", "200", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(cid, "filler", "sendFollowerAddress", "reciveFollowerAddress", config)
		Expect(err).NotTo(HaveOccurred())
		order, err := store.GetOrder(cid)
		Expect(err).NotTo(HaveOccurred())
		Expect(order.ID).To(Equal(cid))
		err = store.CancelOrder("creator", cid)
		Expect(err).To(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("should be able to get all open orders", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		cid1, err := store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:ETH-BTC:BTC", "100", "200", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())

		cid2, err := store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:ETH-BTC:BTC", "200", "300", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())

		cid3, err := store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:ETH-BTC:BTC", "200", "150", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:ETH-BTC:BTC", "200", "400", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())

		orders, err := store.GetActiveOrders()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders)).To(Equal(0))

		err = store.FillOrder(cid1, "filler", "sendFollowerAddress", "reciveFollowerAddress", config)
		Expect(err).NotTo(HaveOccurred())

		orders, err = store.GetActiveOrders()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders)).To(Equal(1))

		err = store.FillOrder(cid2, "filler", "sendFollowerAddress", "reciveFollowerAddress", config)
		Expect(err).NotTo(HaveOccurred())

		orders, err = store.GetActiveOrders()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders)).To(Equal(2))

		err = store.FillOrder(cid3, "filler", "sendFollowerAddress", "reciveFollowerAddress", config)
		Expect(err).NotTo(HaveOccurred())

		// 	orders, err = store.GetActiveOrders()
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(len(orders)).To(Equal(3))
		// 	Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("should be able to get all user's orders", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:WBTC-BTC:BTC", "100", "200", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:WBTC-BTC:BTC", "200", "300", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:WBTC-BTC:BTC", "200", "150", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("creator2", "sendAddress", "receiveAddress", "ETH:WBTC-BTC:BTC", "200", "400", "secretHash", "receivebtcAddress", config)
		Expect(err).NotTo(HaveOccurred())

		// 	unfilledOrders, err := store.FilterOrders("creator", "", "", "", "", model.Status(1), 0, 0, 0, 0, true)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(len(unfilledOrders)).To(Equal(3))

		// 	filledOrders, err := store.FilterOrders("creator", "", "", "", "", model.Status(2), 0, 0, 0, 0, true)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(len(filledOrders)).To(Equal(0))

		// 	filledOrders, err = store.FilterOrders("", "filler", "", "", "", model.Status(2), 0, 0, 0, 0, true)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(len(filledOrders)).To(Equal(0))

		// 	unfilledOrders, err = store.FilterOrders("creator2", "", "", "", "", model.Status(1), 0, 0, 0, 0, true)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(len(unfilledOrders)).To(Equal(1))

		// 	err = store.FillOrder(cid, "filler", "sendFollowerAddress", "reciveFollowerAddress", config)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	unfilledOrders, err = store.FilterOrders("creator", "", "", "", "", model.Status(1), 0, 0, 0, 0, true)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(len(unfilledOrders)).To(Equal(2))

		// 	filledOrders, err = store.FilterOrders("creator", "", "", "", "", model.Status(2), 0, 0, 0, 0, true)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(len(filledOrders)).To(Equal(1))

		// 	filledOrders, err = store.FilterOrders("", "filler", "", "", "", model.Status(2), 0, 0, 0, 0, true)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(len(filledOrders)).To(Equal(1))

		// 	unfilledOrders, err = store.FilterOrders("creator2", "", "", "", "", model.Status(1), 0, 0, 0, 0, true)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(len(unfilledOrders)).To(Equal(1))
		// 	Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})
})
