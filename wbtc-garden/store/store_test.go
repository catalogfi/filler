package store_test

import (
	"crypto/rand"
	"encoding/hex"
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

var config = model.Config{
	Network: model.Network{
		"bitcoin_testnet": model.NetworkConfig{
			Assets: map[model.Asset]model.Token{
				model.Primary: {
					Oracle:   "https://api.coincap.io/v2/assets/bitcoin",
					Decimals: 8,
				},
			},
			RPC:    map[string]string{"mempool": "https://mempool.space/testnet/api"},
			Expiry: 0},
		"ethereum_sepolia": model.NetworkConfig{
			Assets: map[model.Asset]model.Token{
				model.NewSecondary("0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF"): {
					Oracle:       "https://api.coincap.io/v2/assets/bitcoin",
					TokenAddress: "0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF",
					Decimals:     8,
				}},
			RPC:    map[string]string{"ethrpc": "https://gateway.tenderly.co/public/sepolia"},
			Expiry: 0},
	},
	DailyLimit: "350000",
	MinTxLimit: "3000",
	MaxTxLimit: "10000000000",
}

var secretHash string

var _ = BeforeEach(func() {
	secretHashBytes := [32]byte{}
	rand.Read(secretHashBytes[:])
	secretHash = hex.EncodeToString(secretHashBytes[:])
})

var _ = Describe("Store", func() {
	It("should be able to get locked amount", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F872E0F2db", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())

		initiatorUnfilledOrders, err := store.FilterOrders("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "", "", model.Created, 0, 0, 0, 0, 0, 0, true)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(initiatorUnfilledOrders)).Should(BeNumerically(">", 0))

		order := initiatorUnfilledOrders[0]
		order.Status = model.Filled

		store.UpdateOrder(&order)

		followerUnfilledOrders, err := store.FilterOrders("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "", "", model.Status(1), 0, 0, 0, 0, 0, 0, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(followerUnfilledOrders)).Should(BeNumerically(">", 0))

		order = followerUnfilledOrders[0]
		order.Status = model.Executed
		store.UpdateOrder(&order)
		_, err = store.ValueLockedByChain(model.Ethereum, config.Network)
		Expect(err).NotTo(HaveOccurred())

		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})
	It("Error, when using invalid AtomicSwap address", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eD", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
	})

	It("should be able to fill an order", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).To(HaveOccurred())
		order, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())
		Expect(order.Taker).To(Equal("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da"))
		Expect(order.InitiatorAtomicSwap.RedeemerAddress).To(Equal("mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES"))
		Expect(order.FollowerAtomicSwap.InitiatorAddress).To(Equal("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da"))
		// Expect(order.InitiatorAtomicSwap.Timelock).To(Equal(uint64(144)))
		// Expect(order.FollowerAtomicSwap.Timelock).To(Equal(uint64(144)))
		Expect(order.Status).To(Equal(model.Filled))
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("should be able to cancel an order", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		cid, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		order, err := store.GetOrder(cid)
		Expect(err).NotTo(HaveOccurred())
		Expect(order.ID).To(Equal(cid))
		err = store.CancelOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", cid)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.GetOrder(cid)
		Expect(err).To(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	// It("shouldn't be able to cancel a filled order", func() {
	// 	store, err := New(sqlite.Open("test.db"), &gorm.Config{})
	// 	Expect(err).NotTo(HaveOccurred())
	// 	cid, err := store.CreateOrder("creator", "sendAddress", "receiveAddress", "ETH:ETH-BTC:BTC", "100", "200", "secretHash", "receivebtcAddress", config)
	// 	Expect(err).NotTo(HaveOccurred())
	// 	err = store.FillOrder(cid, "filler", "sendFollowerAddress", "reciveFollowerAddress", config.Network)
	// 	Expect(err).NotTo(HaveOccurred())
	// 	order, err := store.GetOrder(cid)
	// 	Expect(err).NotTo(HaveOccurred())
	// 	Expect(order.ID).To(Equal(cid))
	// 	err = store.CancelOrder("creator", cid)
	// 	Expect(err).To(HaveOccurred())
	// 	Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	// })

	It("should be able to get all open orders", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		cid1, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())

		cid2, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F872E0F3dc", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())

		cid3, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F872E0F4dc", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F872E0F5dc", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())

		orders, err := store.GetActiveOrders()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders)).To(Equal(4))

		err = store.FillOrder(cid1, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).NotTo(HaveOccurred())

		orders, err = store.GetActiveOrders()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders)).To(Equal(4))

		err = store.FillOrder(cid2, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).NotTo(HaveOccurred())

		orders, err = store.GetActiveOrders()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders)).To(Equal(4))

		err = store.FillOrder(cid3, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).NotTo(HaveOccurred())

		orders, err = store.GetActiveOrders()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders)).To(Equal(4))
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, shoudl happen cause it crosses daily amount value", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "10000000000000", "1000000000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F872E0F3dc", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F872E0F4dc", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, creating order with amount less than MinTxlimit", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "1000", "1000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())

	})
	It("Error, creating order with amount less than MaxTxlimit", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "10000000000000000000000", "10000000000000000000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())

	})

	It("Error, giving wrong creator address", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58a5B8f9872E0", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, giving wrong unsupported chain format", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "shinto/ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, giving wrong send send chain", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoi_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, giving wrong recive chain", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereu_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, giving wrong send address", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJa", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, giving wrong recieve address", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F8", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, giving wrong recive chain", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, giving wrong send amount", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "1,00000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, giving wrong recieve amount", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "1,00000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, manually changing the daily limit to a wrong value", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		// var config = model.Config{
		// 	Network: model.Network{
		// 		"bitcoin": model.NetworkConfig{RPC: "https://mempool.space/api", Expiry: 0},
		// 		"bitcoin_testnet": model.NetworkConfig{
		// 			Oracles: map[model.Asset]string{
		// 				model.Primary: "https://api.coincap.io/v2/assets/bitcoin",
		// 			},
		// 			RPC:    "https://mempool.space/testnet/api",
		// 			Expiry: 0},
		// 		"ethereum_sepolia": model.NetworkConfig{
		// 			Oracles: map[model.Asset]string{
		// 				model.NewSecondary("0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF"): "https://api.coincap.io/v2/assets/bitcoin",
		// 			},
		// 			RPC:    "https://gateway.tenderly.co/public/sepolia",
		// 			Expiry: 0},
		// 		"ethereum": model.NetworkConfig{RPC: "https://mainnet.infura.io/v3/47b89f1cf0cd47419f9a57674278610b", Expiry: 0},
		// 	},
		// 	DailyLimit: "3,50000",
		// 	MinTxLimit: "3000",
		// 	MaxTxLimit: "10000000000",
		// }
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, manually changing the Min limit to a wrong value", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		// var config = model.Config{
		// 	Network: model.Network{
		// 		"bitcoin": model.NetworkConfig{RPC: "https://mempool.space/api", Expiry: 0},
		// 		"bitcoin_testnet": model.NetworkConfig{
		// 			Oracles: map[model.Asset]string{
		// 				model.Primary: "https://api.coincap.io/v2/assets/bitcoin",
		// 			},
		// 			RPC:    "https://mempool.space/testnet/api",
		// 			Expiry: 0},
		// 		"ethereum_sepolia": model.NetworkConfig{
		// 			Oracles: map[model.Asset]string{
		// 				model.NewSecondary("0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF"): "https://api.coincap.io/v2/assets/bitcoin",
		// 			},
		// 			RPC:    "https://gateway.tenderly.co/public/sepolia",
		// 			Expiry: 0},
		// 		"ethereum": model.NetworkConfig{RPC: "https://mainnet.infura.io/v3/47b89f1cf0cd47419f9a57674278610b", Expiry: 0},
		// 	},
		// 	DailyLimit: "350000",
		// 	MinTxLimit: "3,000",
		// 	MaxTxLimit: "10000000000",
		// }
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, manually changing the daily limit to a wrong value", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		// var config = model.Config{
		// 	Network: model.Network{
		// 		"bitcoin": model.NetworkConfig{RPC: "https://mempool.space/api", Expiry: 0},
		// 		"bitcoin_testnet": model.NetworkConfig{
		// 			Oracles: map[model.Asset]string{
		// 				model.Primary: "https://api.coincap.io/v2/assets/bitcoin",
		// 			},
		// 			RPC:    "https://mempool.space/testnet/api",
		// 			Expiry: 0},
		// 		"ethereum_sepolia": model.NetworkConfig{
		// 			Oracles: map[model.Asset]string{
		// 				model.NewSecondary("0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF"): "https://api.coincap.io/v2/assets/bitcoin",
		// 			},
		// 			RPC:    "https://gateway.tenderly.co/public/sepolia",
		// 			Expiry: 0},
		// 		"ethereum": model.NetworkConfig{RPC: "https://mainnet.infura.io/v3/47b89f1cf0cd47419f9a57674278610b", Expiry: 0},
		// 	},
		// 	DailyLimit: "350000",
		// 	MinTxLimit: "3000",
		// 	MaxTxLimit: "10,000000000",
		// }
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, fill order sender addreses wrong", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).Should(HaveOccurred())

		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, fill order reciever address wrong", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSj", config.Network)
		Expect(err).Should(HaveOccurred())

		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, changing the order after creation", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		order1, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())
		orderp := "bitcoin/optimism"
		order1.OrderPair = orderp
		err = store.UpdateOrder(order1)
		Expect(err).NotTo(HaveOccurred())

		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bc1qznd382dqapcp0j2xf5jyu548g55743jy3ywwqc", config.Network)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, trying to fill an non-existent order", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(5, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, trying to fill an order with wrong config file for sendChain", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		order1, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())
		order1.InitiatorAtomicSwap.Status = 1
		err = store.UpdateOrder(order1)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", config.Network)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())

	})

	It("Error, trying to cancel order by another creator", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.CancelOrder("mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", id)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())

	})
	It("Error, trying to cancel order which doesnt exist", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.CancelOrder("mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", 5)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())

	})

	It("Error, trying to cancel a filled order", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		order1, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())
		order1.Status = 3
		err = store.UpdateOrder(order1)
		Expect(err).NotTo(HaveOccurred())
		err = store.CancelOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", id)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())

	})

	It("Trying to filter order with all the details", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).NotTo(HaveOccurred())
		order1, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())

		order1.InitiatorAtomicSwap.Status = 1
		order1.FollowerAtomicSwap.Status = 2
		err = store.UpdateOrder(order1)
		Expect(err).NotTo(HaveOccurred())

		orders, err := store.FilterOrders("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", secretHash, "", model.Status(2), float64(0.5), float64(10000), float64(0.5), float64(100000), 1, 1, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders)).Should(BeNumerically(">=", 0))
		orders1, err := store.FilterOrders("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", secretHash, "", model.Status(2), float64(0.5), float64(10000), float64(0.5), float64(100000), 0, 0, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders1)).Should(BeNumerically(">=", 0))
		orders2, err := store.FilterOrders("", "", "", "", "-status,-price,id", 1, 0, 0, 0, 0, 0, 0, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(orders2)).Should(BeNumerically(">=", 0))
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())

	})

	It("Error, failed to get send price", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF-bitcoin_testnet", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		order1, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())
		order1.InitiatorAtomicSwap.Status = 1
		err = store.UpdateOrder(order1)
		Expect(err).NotTo(HaveOccurred())

		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", config.Network)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, failed to get recieve price", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF-bitcoin_testnet", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		order1, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())
		order1.InitiatorAtomicSwap.Status = 1
		err = store.UpdateOrder(order1)
		Expect(err).NotTo(HaveOccurred())

		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", config.Network)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Error, if Amount is corrupted", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF-bitcoin_testnet", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		order1, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())
		order1.InitiatorAtomicSwap.Status = 1
		order1.FollowerAtomicSwap.Status = 1
		order1.InitiatorAtomicSwap.Amount = "10,0000"
		order1.FollowerAtomicSwap.Amount = "10,0000"

		err = store.UpdateOrder(order1)
		Expect(err).NotTo(HaveOccurred())

		err = store.FillOrder(id, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", config.Network)
		Expect(err).Should(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})
	It("Error, creating order with wrong config file", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF-bitcoin_testnet", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", model.Config{})
		Expect(err).Should(HaveOccurred())

		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
	})

	It("Creating filling and then creating with same address", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id1, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		err = store.FillOrder(id1, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F873E0F2dc", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())

	})

	It("Error, deleting database after creating and then trying to fill", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id1, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())

		err = store.FillOrder(id1, "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config.Network)
		Expect(err).Should(HaveOccurred())

	})

	It("Error, creating order in a deleted database", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
		_, err = store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", "17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2daE6B5ca5B8f9Ec6F873E0F2dc", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).Should(HaveOccurred())

	})

	It("Error, updating order in a deleted database", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		order1, err := store.GetOrder(id)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
		orderp := "bitcoin/optimism"
		order1.OrderPair = orderp
		err = store.UpdateOrder(order1)
		Expect(err).Should(HaveOccurred())
	})

	It("Error, attempting to cancel order when Db is deleted", func() {
		store, err := New(sqlite.Open("test.db"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())
		id, err := store.CreateOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", "0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF", "100000000", "100000000", secretHash, "mg54DDo5jfNkx5tF4d7Ag6G6VrJaSjr7ES", config)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Remove("test.db")).NotTo(HaveOccurred())
		err = store.CancelOrder("0x17100301bB2FF58aE6B5ca5B8f9Ec6F872E0F2da", id)
		Expect(err).Should(HaveOccurred())

	})

})
