package executor_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/fatih/color"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/blockchain/btc/btctest"
	"github.com/catalogfi/blockchain/testutil"
	"github.com/catalogfi/cobi/pkg/cobid/executor"
	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
)

// type Order struct {
// 	gorm.Model

// 	Maker     string `json:"maker"`
// 	Taker     string `json:"taker"`
// 	OrderPair string `json:"orderPair"`

// 	InitiatorAtomicSwapID uint
// 	FollowerAtomicSwapID  uint
// 	InitiatorAtomicSwap   *AtomicSwap `json:"initiatorAtomicSwap" gorm:"foreignKey:InitiatorAtomicSwapID"`
// 	FollowerAtomicSwap    *AtomicSwap `json:"followerAtomicSwap" gorm:"foreignKey:FollowerAtomicSwapID"`

// 	SecretHash           string  `json:"secretHash" gorm:"unique;not null"`
// 	Secret               string  `json:"secret"`
// 	Price                float64 `json:"price"`
// 	Status               Status  `json:"status"`
// 	SecretNonce          uint64  `json:"secretNonce"`
// 	UserBtcWalletAddress string  `json:"userBtcWalletAddress"`
// 	RandomMultiplier     uint64
// 	RandomScore          uint64

// 	Fee uint `json:"fee"`
// }

// type AtomicSwap struct {
// 	gorm.Model

//		Status               SwapStatus `json:"swapStatus"`
//		SecretHash           string     `json:"secretHash"`
//		Secret               string     `json:"secret"`
//		OnChainIdentifier    string     `json:"onChainIdentifier"`
//		InitiatorAddress     string     `json:"initiatorAddress"`
//		RedeemerAddress      string     `json:"redeemerAddress"`
//		Timelock             string     `json:"timelock"`
//		Chain                Chain      `json:"chain"`
//		Asset                Asset      `json:"asset"`
//		Amount               string     `json:"amount"`
//		FilledAmount         string     `json:"filledAmount"`
//		InitiateTxHash       string     `json:"initiateTxHash" `
//		RedeemTxHash         string     `json:"redeemTxHash" `
//		RefundTxHash         string     `json:"refundTxHash" `
//		PriceByOracle        float64    `json:"priceByOracle"`
//		MinimumConfirmations uint64     `json:"minimumConfirmations"`
//		CurrentConfirmations uint64     `json:"currentConfirmation"`
//		InitiateBlockNumber  uint64     `json:"initiateBlockNumber"`
//		IsInstantWallet      bool       `json:"-"`
//	}

func generateOrder(
	id uint,
	initiatorInitAddr, initiatorRedeemAddr, followerInitAddr, followerRedeemAddr, maker, taker, orderPair string,
	initSwapStatus, followerSwapStatus model.SwapStatus,
	orderStatus model.Status,
	initTL, followerTL string,
	amount *big.Int,
	secret []byte) model.Order {

	initChain, followerChain, initAsset, followerAsset, err := model.ParseOrderPair(orderPair)
	if err != nil {
		log.Fatalf("%v", err)
	}

	shBytes := sha256.Sum256(secret)
	secretHash := hex.EncodeToString(shBytes[:])
	order := model.Order{
		Maker:     maker,
		Taker:     taker,
		OrderPair: orderPair,
		InitiatorAtomicSwap: &model.AtomicSwap{
			Status:           initSwapStatus,
			SecretHash:       secretHash,
			Secret:           hex.EncodeToString(secret),
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
			Secret:           hex.EncodeToString(secret),
			InitiatorAddress: followerInitAddr,
			RedeemerAddress:  initiatorRedeemAddr,
			Timelock:         followerTL,
			Chain:            followerChain,
			Asset:            followerAsset,
			Amount:           amount.String(),
		},
		SecretHash: secretHash,
		Secret:     hex.EncodeToString(secret),
		Status:     orderStatus,
	}
	order.ID = id
	return order

}

var _ = Describe("Executor_setup", Ordered, func() {
	var exec executor.Executor
	var cobiEthWallet ethswap.Wallet
	var aliceEthWallet ethswap.Wallet
	var cobiBtcWallet btcswap.Wallet
	var aliceBtcWallet btcswap.Wallet
	var evmclient *ethclient.Client
	var execstore *store.Store
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

		obclient := rest.NewWSClient(fmt.Sprintf("ws://%s/", orderBookUrl), logger.With(zap.String("client", "orderbook")))

		quit := make(chan struct{})

		os.Remove("test.db")
		db, err := gorm.Open(sqlite.Open("test.db"))
		Expect(err).To(BeNil())

		store, err := store.NewStore(db)
		execstore = &store
		Expect(err).To(BeNil())

		exec = executor.NewExecutor(cobiBtcWallet, cobiEthWallet, cobiEthWallet.Address(), obclient, executor.RegtestOptions(orderBookUrl), store, logger, quit)

		go func() {
			exec.Start()
		}()
	})
	AfterAll(func() {
		exec.Stop()
	})
	Context("wbtc to btc trade", func() {
		It("cobi should initiate btc", func(ctx context.Context) {

			By("Alice constructs a swap")
			amount := big.NewInt(1e7)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			expiry := big.NewInt(6)
			eswap, err := ethswap.NewSwap(aliceEthWallet.Address(), cobiEthWallet.Address(), swapAddr, secretHash, amount, expiry)
			Expect(err).To(BeNil())
			bswap, err := btcswap.NewSwap(&chaincfg.RegressionNetParams, cobiBtcWallet.Address(), aliceBtcWallet.Address(), amount.Int64(), secretHash[:], 6)
			Expect(err).To(BeNil())

			By("Check status")
			initiated, err := eswap.Initiated(ctx, evmclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			redeemed, err := eswap.Redeemed(ctx, evmclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice initiates the swap")
			initTx, err := aliceEthWallet.Initiate(ctx, eswap)
			Expect(err).To(BeNil())
			By(color.GreenString("Initiation tx hash = %v", initTx))
			time.Sleep(time.Second)

			By("Check status")
			initiated, err = eswap.Initiated(ctx, evmclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			redeemed, err = eswap.Redeemed(ctx, evmclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			//generate random number oid uint

			By("Creating an order")
			oid := rand.Intn(100)
			err = (*execstore).PutSecret(hex.EncodeToString(secretHash[:]), nil, uint64(oid))
			Expect(err).To(BeNil())

			order := generateOrder(
				uint(oid),
				aliceEthWallet.Address().Hex(), aliceBtcWallet.Address().EncodeAddress(),
				cobiBtcWallet.Address().EncodeAddress(), cobiEthWallet.Address().Hex(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr),
				model.Initiated, model.NotStarted,
				model.Filled,
				expiry.String(), expiry.String(), amount, secret)

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			err = testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			isInit, _, err := bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(isInit).Should(BeTrue())

		})
		It("cobi should redeem wbtc", func() {

		})
		It("cobi should refund btc", func() {

		})
	})

	Context("btc to wbtc trade", func() {
		It("cobi should initiate wbtc", func() {

		})
		It("cobi should redeem btc", func() {

		})
		It("cobi should refund wbtc", func() {

		})
	})

})
