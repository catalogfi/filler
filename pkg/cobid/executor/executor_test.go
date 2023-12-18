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
	"go.uber.org/zap/zaptest/observer"
)

func setupLogsCapture() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.InfoLevel)
	return zap.New(core), logs
}
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

var _ = Describe("Executor", Ordered, func() {
	var exec executor.Executor
	var cobiEthWallet ethswap.Wallet
	var aliceEthWallet ethswap.Wallet
	var cobiBtcWallet btcswap.Wallet
	var aliceBtcWallet btcswap.Wallet
	var evmclient *ethclient.Client
	var execstore *store.Store
	var btcclient btc.IndexerClient
	var observer *observer.ObservedLogs
	BeforeAll(func() {
		orderBookUrl := "localhost:8080"

		var err error

		//btc wallet setup
		network := &chaincfg.RegressionNetParams
		btcclient = btctest.RegtestIndexer()
		cobiBtcWallet, err = NewTestWallet(network, btcclient)
		Expect(err).To(BeNil())
		//this ensure the bitcoin is atually funded before
		_, err = testutil.NigiriFaucet(cobiBtcWallet.Address().EncodeAddress())
		Expect(err).To(BeNil())
		err = testutil.NigiriNewBlock()
		Expect(err).To(BeNil())
		time.Sleep(5 * time.Second)

		bobBtcBalance, err := cobiBtcWallet.Balance(context.Background(), true)
		Expect(err).To(BeNil())
		Expect(bobBtcBalance).To(BeNumerically("==", 100000000))

		aliceBtcWallet, err = NewTestWallet(network, btcclient)
		Expect(err).To(BeNil())

		_, err = testutil.NigiriFaucet(aliceBtcWallet.Address().EncodeAddress())
		Expect(err).To(BeNil())
		err = testutil.NigiriNewBlock()
		Expect(err).To(BeNil())
		time.Sleep(5 * time.Second)

		aliceBtcBalance, err := aliceBtcWallet.Balance(context.Background(), true)
		Expect(err).To(BeNil())
		Expect(aliceBtcBalance).To(BeNumerically("==", 100000000))

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

		var logger *zap.Logger
		logger, observer = setupLogsCapture()
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
		var eswap *ethswap.Swap
		var bswap btcswap.Swap
		var secret []byte
		var secretHash [32]byte
		var expiry *big.Int
		var amount *big.Int
		var oid int
		var orderPair string

		BeforeAll(func() {
			Skip("skip")
			var err error
			orderPair = fmt.Sprintf("ethereum_localnet:%s-bitcoin_regtest", tokenAddr)
			//generating random number order id
			oid = rand.Intn(100000)
			amount = big.NewInt(1e7)
			secret = testutil.RandomSecret()
			secretHash = sha256.Sum256(secret)
			expiry = big.NewInt(6)
			eswap, err = ethswap.NewSwap(aliceEthWallet.Address(), cobiEthWallet.Address(), swapAddr, secretHash, amount, expiry)
			Expect(err).To(BeNil())
			bswap, err = btcswap.NewSwap(&chaincfg.RegressionNetParams, cobiBtcWallet.Address(), aliceBtcWallet.Address(), amount.Int64(), secretHash[:], 6)
			Expect(err).To(BeNil())
		})
		It("cobi should initiate btc", func(ctx context.Context) {
			var err error

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

			By("sending an order via socket message")
			//generating random number order id
			err = (*execstore).PutSecret(hex.EncodeToString(secretHash[:]), nil, uint64(oid))
			Expect(err).To(BeNil())

			order := generateOrder(
				uint(oid),
				aliceEthWallet.Address().Hex(), aliceBtcWallet.Address().EncodeAddress(),
				cobiBtcWallet.Address().EncodeAddress(), cobiEthWallet.Address().Hex(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				orderPair,
				model.Initiated, model.NotStarted,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)
			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "initiate tx hash")).Should(BeTrue())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())
			err = testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			isInit, _, err := bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(isInit).Should(BeTrue())

		})
		It("cobi should redeem wbtc", func(ctx context.Context) {
			order := generateOrder(
				uint(oid),
				aliceEthWallet.Address().Hex(), aliceBtcWallet.Address().EncodeAddress(),
				cobiBtcWallet.Address().EncodeAddress(), cobiEthWallet.Address().Hex(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				orderPair,
				model.Initiated, model.Initiated,
				model.Filled,
				expiry.String(), expiry.String(), amount, hex.EncodeToString(secret), hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			err := testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)
			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "redeem tx hash")).Should(BeTrue())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())

			isRedeemed, err := (*eswap).Redeemed(ctx, evmclient)
			Expect(err).To(BeNil())
			Expect(isRedeemed).Should(BeTrue())

		})
		It("cobi should refund btc", func(ctx context.Context) {
			// TODO: bitcoin refund doesnot return a tx hash
			var err error
			oid := rand.Intn(100000)
			amount := big.NewInt(1e7)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			expiry := big.NewInt(1)
			eswap, err := ethswap.NewSwap(aliceEthWallet.Address(), cobiEthWallet.Address(), swapAddr, secretHash, amount, expiry)
			Expect(err).To(BeNil())
			bswap, err := btcswap.NewSwap(&chaincfg.RegressionNetParams, cobiBtcWallet.Address(), aliceBtcWallet.Address(), amount.Int64(), secretHash[:], 1)
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

			By("sending an order via socket message")
			err = (*execstore).PutSecret(hex.EncodeToString(secretHash[:]), nil, uint64(oid))
			Expect(err).To(BeNil())

			order := generateOrder(
				uint(oid),
				aliceEthWallet.Address().Hex(), aliceBtcWallet.Address().EncodeAddress(),
				cobiBtcWallet.Address().EncodeAddress(), cobiEthWallet.Address().Hex(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				orderPair,
				model.Initiated, model.NotStarted,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			err = testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			err = testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			isInit, _, err := bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(isInit).Should(BeTrue())

			order = generateOrder(
				uint(oid),
				aliceEthWallet.Address().Hex(), aliceBtcWallet.Address().EncodeAddress(),
				cobiBtcWallet.Address().EncodeAddress(), cobiEthWallet.Address().Hex(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				orderPair,
				model.Initiated, model.Expired,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			err = testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "refund tx hash")).Should(BeTrue())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())
			utxos, err := btcclient.GetUTXOs(ctx, bswap.Address)
			Expect(err).To(BeNil())
			Expect(len(utxos)).Should(Equal(0))

		})
	})

	Context("btc to wbtc trade", func() {
		var eswap *ethswap.Swap
		var bswap btcswap.Swap
		var secret []byte
		var secretHash [32]byte
		var expiry *big.Int
		var amount *big.Int
		var oid int
		var orderPair string

		BeforeAll(func() {
			Skip("skip")
			var err error
			orderPair = fmt.Sprintf("bitcoin_regtest-ethereum_localnet:%s", tokenAddr)
			//generating random number order id
			oid = rand.Intn(100000)
			amount = big.NewInt(1e7)
			secret = testutil.RandomSecret()
			secretHash = sha256.Sum256(secret)
			expiry = big.NewInt(6)
			eswap, err = ethswap.NewSwap(cobiEthWallet.Address(), aliceEthWallet.Address(), swapAddr, secretHash, amount, expiry)
			Expect(err).To(BeNil())
			bswap, err = btcswap.NewSwap(&chaincfg.RegressionNetParams, aliceBtcWallet.Address(), cobiBtcWallet.Address(), amount.Int64(), secretHash[:], 6)
			Expect(err).To(BeNil())
		})

		It("cobi should initiate wbtc", func(ctx context.Context) {
			var err error

			By("Check status")
			initiated, _, err := bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			redeemed, _, err := bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice initiates the swap")
			initTx, err := aliceBtcWallet.Initiate(ctx, bswap)
			Expect(err).To(BeNil())
			By(color.GreenString("Initiation tx hash = %v", initTx))
			time.Sleep(time.Second)

			err = testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			By("Check status")
			initiated, _, err = bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			redeemed, _, err = bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("sending an order via socket message")
			//generating random number order id
			err = (*execstore).PutSecret(hex.EncodeToString(secretHash[:]), nil, uint64(oid))
			Expect(err).To(BeNil())

			order := generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				orderPair,
				model.Initiated, model.NotStarted,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)
			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "initiate tx hash")).Should(BeTrue())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())

			isInit, err := eswap.Initiated(ctx, evmclient)
			Expect(err).To(BeNil())
			Expect(isInit).Should(BeTrue())

		})
		It("cobi should redeem btc", func(ctx context.Context) {
			order := generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				orderPair,
				model.Initiated, model.Initiated,
				model.Filled,
				expiry.String(), expiry.String(), amount, hex.EncodeToString(secret), hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			err := testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "redeem tx hash")).Should(BeTrue())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())
			isRedeemed, _, err := bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(isRedeemed).Should(BeTrue())

		})
		It("cobi should refund wbtc", func(ctx context.Context) {
			var err error
			oid := rand.Intn(100000)
			amount := big.NewInt(1e7)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			expiry := big.NewInt(1)
			eswap, err := ethswap.NewSwap(cobiEthWallet.Address(), aliceEthWallet.Address(), swapAddr, secretHash, amount, expiry)
			Expect(err).To(BeNil())
			bswap, err := btcswap.NewSwap(&chaincfg.RegressionNetParams, aliceBtcWallet.Address(), cobiBtcWallet.Address(), amount.Int64(), secretHash[:], 1)
			Expect(err).To(BeNil())

			By("Check status")
			initiated, _, err := bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			redeemed, _, err := bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice initiates the swap")
			initTx, err := aliceBtcWallet.Initiate(ctx, bswap)
			Expect(err).To(BeNil())
			By(color.GreenString("Initiation tx hash = %v", initTx))
			time.Sleep(time.Second)

			err = testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			By("Check status")
			initiated, _, err = bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			redeemed, _, err = bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("sending an order via socket message")
			//generating random number order id
			err = (*execstore).PutSecret(hex.EncodeToString(secretHash[:]), nil, uint64(oid))
			Expect(err).To(BeNil())

			order := generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				orderPair,
				model.Initiated, model.NotStarted,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			isInit, err := eswap.Initiated(ctx, evmclient)
			Expect(err).To(BeNil())
			Expect(isInit).Should(BeTrue())

			order = generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				orderPair,
				model.Initiated, model.Expired,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "refund tx hash")).Should(BeTrue())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())

			// TODO: add refund check in wallet
			// isRefunded, _, err := eswap.Refunded(ctx, btcclient)
			// Expect(err).To(BeNil())
			// Expect(isRefunded).Should(BeTrue())

		})
	})
	Context("Re-execution tests", func() {
		var eswap *ethswap.Swap
		var bswap btcswap.Swap
		var secret []byte
		var secretHash [32]byte
		var expiry *big.Int
		var amount *big.Int
		var oid int
		var orderPair string

		BeforeAll(func() {
			var err error
			orderPair = fmt.Sprintf("bitcoin_regtest-ethereum_localnet:%s", tokenAddr)
			//generating random number order id
			oid = rand.Intn(100000)
			amount = big.NewInt(1e7)
			secret = testutil.RandomSecret()
			secretHash = sha256.Sum256(secret)
			expiry = big.NewInt(6)
			eswap, err = ethswap.NewSwap(cobiEthWallet.Address(), aliceEthWallet.Address(), swapAddr, secretHash, amount, expiry)
			Expect(err).To(BeNil())
			bswap, err = btcswap.NewSwap(&chaincfg.RegressionNetParams, aliceBtcWallet.Address(), cobiBtcWallet.Address(), amount.Int64(), secretHash[:], 6)
			Expect(err).To(BeNil())
		})

		It("cobi should initiate wbtc", func(ctx context.Context) {
			var err error

			By("Check status")
			initiated, _, err := bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			redeemed, _, err := bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice initiates the swap")
			initTx, err := aliceBtcWallet.Initiate(ctx, bswap)
			Expect(err).To(BeNil())
			By(color.GreenString("Initiation tx hash = %v", initTx))
			time.Sleep(time.Second)

			err = testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			By("Check status")
			initiated, _, err = bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			redeemed, _, err = bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("sending an order via socket message")
			//generating random number order id
			err = (*execstore).PutSecret(hex.EncodeToString(secretHash[:]), nil, uint64(oid))
			Expect(err).To(BeNil())

			order := generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				orderPair,
				model.Initiated, model.NotStarted,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			isInit, err := eswap.Initiated(ctx, evmclient)
			Expect(err).To(BeNil())
			Expect(isInit).Should(BeTrue())
			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "initiate tx hash")).Should(BeTrue())

		})
		It("cobi should not re-initiate wbtc", func(ctx context.Context) {
			order := generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				orderPair,
				model.Initiated, model.NotStarted,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)
			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "initiate")).Should(BeFalse())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())
		})
		It("cobi should redeem btc", func(ctx context.Context) {
			order := generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				orderPair,
				model.Initiated, model.Initiated,
				model.Filled,
				expiry.String(), expiry.String(), amount, hex.EncodeToString(secret), hex.EncodeToString(secretHash[:]))
			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			err := testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			isRedeemed, _, err := bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(isRedeemed).Should(BeTrue())
			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "redeem tx hash")).Should(BeTrue())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())

		})
		It("cobi should not re-initiate wbtc after redeeming btc", func(ctx context.Context) {
			order := generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				orderPair,
				model.Initiated, model.NotStarted,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)
			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "initiate")).Should(BeFalse())
		})
		It("cobi should not redeem btc twice", func(ctx context.Context) {
			order := generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				orderPair,
				model.Initiated, model.Initiated,
				model.Filled,
				expiry.String(), expiry.String(), amount, hex.EncodeToString(secret), hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			err := testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			isRedeemed, _, err := bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(isRedeemed).Should(BeTrue())
			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "redeem")).Should(BeFalse())
		})
		It("cobi should not refund wbtc twice", func(ctx context.Context) {
			var err error
			oid := rand.Intn(100000)
			amount := big.NewInt(1e7)
			secret := testutil.RandomSecret()
			secretHash := sha256.Sum256(secret)
			expiry := big.NewInt(1)
			eswap, err := ethswap.NewSwap(cobiEthWallet.Address(), aliceEthWallet.Address(), swapAddr, secretHash, amount, expiry)
			Expect(err).To(BeNil())
			bswap, err := btcswap.NewSwap(&chaincfg.RegressionNetParams, aliceBtcWallet.Address(), cobiBtcWallet.Address(), amount.Int64(), secretHash[:], 1)
			Expect(err).To(BeNil())

			By("Check status")
			initiated, _, err := bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeFalse())
			redeemed, _, err := bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("Alice initiates the swap")
			initTx, err := aliceBtcWallet.Initiate(ctx, bswap)
			Expect(err).To(BeNil())
			By(color.GreenString("Initiation tx hash = %v", initTx))
			time.Sleep(time.Second)

			err = testutil.NigiriNewBlock()
			Expect(err).To(BeNil())
			time.Sleep(5 * time.Second)

			By("Check status")
			initiated, _, err = bswap.Initiated(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(initiated).Should(BeTrue())
			redeemed, _, err = bswap.Redeemed(ctx, btcclient)
			Expect(err).To(BeNil())
			Expect(redeemed).Should(BeFalse())

			By("sending an order via socket message")
			//generating random number order id
			err = (*execstore).PutSecret(hex.EncodeToString(secretHash[:]), nil, uint64(oid))
			Expect(err).To(BeNil())

			order := generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				orderPair,
				model.Initiated, model.NotStarted,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			isInit, err := eswap.Initiated(ctx, evmclient)
			Expect(err).To(BeNil())
			Expect(isInit).Should(BeTrue())

			order = generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				orderPair,
				model.Initiated, model.Expired,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}

			By("waiting for executor")
			time.Sleep(5 * time.Second)

			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "refund tx hash")).Should(BeTrue())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())

			// TODO: add refund check in wallet
			// isRefunded, _, err := eswap.Refunded(ctx, btcclient)
			// Expect(err).To(BeNil())
			// Expect(isRefunded).Should(BeTrue())

			order = generateOrder(
				uint(oid),
				aliceBtcWallet.Address().EncodeAddress(), aliceEthWallet.Address().Hex(),
				cobiEthWallet.Address().Hex(), cobiBtcWallet.Address().EncodeAddress(),
				aliceEthWallet.Address().Hex(), cobiEthWallet.Address().Hex(),
				orderPair,
				model.Initiated, model.Expired,
				model.Filled,
				expiry.String(), expiry.String(), amount, "", hex.EncodeToString(secretHash[:]))

			server.Msg <- rest.UpdatedOrders{
				Orders: []model.Order{order},
				Error:  "",
			}
			By("waiting for executor")
			time.Sleep(5 * time.Second)

			Expect(strings.Contains(observer.All()[len(observer.All())-1].Message, "refund")).Should(BeFalse())
			Expect(observer.All()[len(observer.All())-1].Level == zap.InfoLevel).Should(BeTrue())

		})
	})
})
