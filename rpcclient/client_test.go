package rpcclient_test

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	jsonrpc "github.com/catalogfi/cobi/daemon/rpc"
	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/rpcclient"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/orderbook/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	s            jsonrpc.RPC
	c            rpcclient.Client
	CurrentOrder uint64
)

var _ = BeforeSuite(func() {
	filePath := "test.db"
	if _, err := os.Stat(filePath); err == nil {
		// If file exists, then remove
		os.Remove(filePath)
	}

	StartServer()
	time.Sleep(3 * time.Second) // await server to start
	c = rpcclient.NewClient("admin", "pass", "http", "127.0.0.1:3000")
})

var _ = Describe("ClientTesting", func() {
	It("Get Accounts without InstantWallet", func() {
		AccountReq := types.RequestAccount{
			IsInstantWallet: false,
			Asset:           "bitcoin_testnet",
			Page:            uint32(1),
			PerPage:         uint32(10),
			UserAccount:     0,
		}

		resp, err := c.GetAccounts(AccountReq)
		Expect(err).To(BeNil())

		var accounts []types.AccountInfo
		if err := json.Unmarshal(resp, &accounts); err != nil {
			Expect(err).To(BeNil())
		}

		Expect(accounts).NotTo(BeEmpty())
		Expect(len(accounts)).To(Equal(10))
		Expect(accounts[0].AccountNo).To(Equal("0"))
	})

	It("Get Accounts with InstantWallet", func() {
		// Skip("not working")
		AccountReq := types.RequestAccount{
			IsInstantWallet: true,
			Asset:           "bitcoin_testnet",
			Page:            uint32(1),
			PerPage:         uint32(10),
			UserAccount:     0,
		}

		resp, err := c.GetAccounts(AccountReq)
		Expect(err).To(BeNil())

		var accounts []types.AccountInfo
		if err := json.Unmarshal(resp, &accounts); err != nil {
			Expect(err).To(BeNil())
		}

		Expect(accounts).NotTo(BeEmpty())
		Expect(len(accounts)).To(Equal(10))
		Expect(accounts[0].AccountNo).To(Equal("0"))
	})

	It("Create Order", func() {
		Skip("")

		CreateOrder := types.RequestCreate{
			UserAccount:   0,
			OrderPair:     "bitcoin_testnet-ethereum_sepolia:0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF",
			SendAmount:    "1000",
			ReceiveAmount: "2000",
		}

		resp, err := c.CreateOrder(CreateOrder)
		Expect(err).To(BeNil())

		var OrderId uint64
		if err := json.Unmarshal(resp, &OrderId); err != nil {
			Expect(err).To(BeNil())
		}

		Expect(OrderId).NotTo(BeZero())
		CurrentOrder = OrderId
	})

	It("FillOder", func() {
		Skip("")

		FillOrder := types.RequestFill{
			UserAccount: 1,
			OrderId:     CurrentOrder,
		}

		_, err := c.FillOrder(FillOrder)
		Expect(err).To(BeNil())

	})

	It("Deposit to Instant Wallet", func() {
		Skip("")

		Deposit := types.RequestDeposit{
			UserAccount: 1,
			Amount:      10000,
			Asset:       "bitcoin_testnet",
		}

		_, err := c.Deposit(Deposit)
		Expect(err).To(BeNil())
	})

	It("Transfer Funds", func() {
		Skip("")
		Transfer := types.RequestTransfer{
			UserAccount: 0,
			Asset:       "bitcoin_testnet",
			Amount:      1000,
			ToAddr:      "tb1qdq0gsaawa6cy049xq2yd9jfpskmgfkt2vkgxqd",
			UseIw:       false,
			Force:       false,
		}

		_, err := c.Transfer(Transfer)
		Expect(err).To(BeNil())
	})

	It("Get List Of Orders", func() {
		Skip("Testging setConfig")
		resp, err := c.ListOrders(types.RequestListOrders{
			Page:    1,
			PerPage: 10,
		})
		Expect(err).To(BeNil())

		var orders []model.Order
		if err := json.Unmarshal(resp, &orders); err != nil {
			Expect(err).To(BeNil())
		}

		Expect(orders).NotTo(BeEmpty())
		fmt.Println("len", len(orders))
		// Expect(len(orders)).To(Equal(10))

	})

	It("Should SetConfig and verify Config State Has Changed in server", func() {
		AccountReq := types.RequestAccount{
			IsInstantWallet: true,
			Asset:           "bitcoin_testnet",
			Page:            uint32(1),
			PerPage:         uint32(10),
			UserAccount:     0,
		}

		resp, err := c.SetConfig(types.SetConfig{
			RpcUserName: "rpcuser",
			RpcPassword: "rpcpass",
		})
		Expect(err).To(BeNil())
		fmt.Println("resp", string(resp))

		c.UpdateAuth("admin", "pass")
		resp, err = c.GetAccounts(AccountReq)
		Expect(err).ToNot(BeNil())

		c.UpdateAuth("Malicious_admin", "pass")
		resp, err = c.GetAccounts(AccountReq)
		Expect(err).ToNot(BeNil())

		c.UpdateAuth("admin", "wrong_pass")
		resp, err = c.GetAccounts(AccountReq)
		Expect(err).ToNot(BeNil())

		c.UpdateAuth("rpcuser", "rpcpass")
		resp, err = c.GetAccounts(AccountReq)
		Expect(err).To(BeNil())
		Expect(resp).ToNot(BeNil())
	})
})

func StartServer() {
	go func() {
		envConfig, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
		if err != nil {
			panic(err)
		}

		var str store.Store
		if envConfig.DB != "" {
			// Initialise db
			str, err = store.NewStore(sqlite.Open(envConfig.DB), &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
			})
			if err != nil {
				panic(err)
			}
		} else {
			str, err = store.NewStore(sqlite.Open(utils.DefaultStorePath()), &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
			})
			if err != nil {
				panic(err)
			}
		}

		entropy, err := bip39.EntropyFromMnemonic(envConfig.Mnemonic)
		if err != nil {
			panic(err)
		}

		// Load keys
		keys := utils.NewKeys(entropy)

		logger, err := zap.NewProduction()
		if err != nil {
			panic(err)
		}
		s = jsonrpc.NewRpcServer(str, envConfig, &keys, logger)
		s.Run()
	}()
}
