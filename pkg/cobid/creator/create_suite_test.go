package creator_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/blockchain/btc/btctest"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap/bindings"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest/utils"
	"github.com/dgrijalva/jwt-go"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/fatih/color"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spruceid/siwe-go"
	"go.uber.org/zap"
)

func TestCreator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Creator Suite")
}

var (
	swapAddr  common.Address
	tokenAddr common.Address
	server    *TestOrderBookServer
	Cancel    context.CancelFunc
)

var _ = BeforeSuite(func() {
	By("Required envs")
	Expect(os.Getenv("ETH_URL")).ShouldNot(BeEmpty())
	Expect(os.Getenv("ETH_KEY_1")).ShouldNot(BeEmpty())
	Expect(os.Getenv("ETH_KEY_2")).ShouldNot(BeEmpty())

	By("Initialise client")
	url := os.Getenv("ETH_URL")
	client, err := ethclient.Dial(url)
	Expect(err).Should(BeNil())
	chainID, err := client.ChainID(context.Background())
	Expect(err).Should(BeNil())

	// need to deploy ERC20 and AtomicSwap contracts in order to create ETH wallets
	By("Initialise transactor")
	keyStr := strings.TrimPrefix(os.Getenv("ETH_KEY_1"), "0x")
	keyBytes, err := hex.DecodeString(keyStr)
	Expect(err).Should(BeNil())
	key, err := crypto.ToECDSA(keyBytes)
	Expect(err).Should(BeNil())
	transactor, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	Expect(err).Should(BeNil())

	By("Deploy ERC20 contract")
	var tx *types.Transaction
	tokenAddr, tx, _, err = bindings.DeployTestERC20(transactor, client)
	Expect(err).Should(BeNil())
	_, err = bind.WaitMined(context.Background(), client, tx)
	Expect(err).Should(BeNil())
	By(color.GreenString("ERC20 deployed to %v", tokenAddr.Hex()))

	By("Deploy atomic swap contract")
	swapAddr, tx, _, err = bindings.DeployAtomicSwap(transactor, client, tokenAddr)
	Expect(err).Should(BeNil())
	_, err = bind.WaitMined(context.Background(), client, tx)
	Expect(err).Should(BeNil())
	By(color.GreenString("Atomic swap deployed to %v", swapAddr.Hex()))

	var ctx context.Context
	logger, err := zap.NewDevelopment()
	Expect(err).To(BeNil())
	ctx, Cancel = context.WithCancel(context.Background())
	server = NewTestServer(logger)

	go func() {
		server.Run(ctx, ":8080")
	}()
})

var _ = AfterSuite(func() {
	Cancel()
	fmt.Println("Server Stopped")
})

func NewTestWallet(network *chaincfg.Params, client btc.IndexerClient) (btcswap.Wallet, error) {
	key, _, err := btctest.NewBtcKey(network)
	if err != nil {
		return nil, err
	}
	opts := btcswap.OptionsRegression()
	fee := rand.Intn(18) + 2
	feeEstimator := btc.NewFixFeeEstimator(fee)
	return btcswap.NewWallet(opts, client, key, feeEstimator)
}

type TestOrderBookServer struct {
	router *gin.Engine
	logger *zap.Logger
	Msg    chan interface{}
}

func NewTestServer(logger *zap.Logger) *TestOrderBookServer {
	childLogger := logger.With(zap.String("service", "rest"))
	return &TestOrderBookServer{
		router: gin.Default(),
		logger: childLogger,
		Msg:    make(chan interface{}),
	}
}

func (s *TestOrderBookServer) Run(ctx context.Context, addr string) error {
	s.router.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	s.router.GET("/nonce", s.nonce())
	s.router.POST("/verify", s.verify()) // SIWE VERIFY

	authRoutes := s.router.Group("/")
	authRoutes.Use(authenticate)
	{
		authRoutes.POST("/orders", s.postOrders())
	}

	service := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	go func() {
		if err := service.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
		s.logger.Info("stopped")
	}()
	<-ctx.Done()
	return service.Shutdown(ctx)
}

func (s *TestOrderBookServer) nonce() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"nonce": siwe.GenerateNonce(),
		})
	}
}

func (s *TestOrderBookServer) verify() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		req := model.VerifySiwe{}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		token, err := Verify(req)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		tokenString, err := token.SignedString([]byte("SECRET"))
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{"token": tokenString})
	}
}

type Claims struct {
	UserWallet string `json:"userWallet"`
	jwt.StandardClaims
}

func Verify(req model.VerifySiwe) (*jwt.Token, error) {
	parsedMessage, err := siwe.ParseMessage(req.Message)
	if err != nil {
		return nil, fmt.Errorf("Error parsing message: %w ", err)
	}

	valid, err := parsedMessage.ValidNow()
	if err != nil {
		return nil, fmt.Errorf("Error validating message: %w ", err)
	}
	if !valid {
		return nil, fmt.Errorf("Validating expired Token")
	}

	fromAddress, err := verifySignature(parsedMessage.String(), req.Signature, parsedMessage.GetAddress(), parsedMessage.GetChainID())

	if err != nil {
		return nil, fmt.Errorf("Error verifying message: %w ", err)
	}

	claims := &Claims{
		UserWallet: strings.ToLower(fromAddress.String()),
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour * 24).Unix(), // Token expires in 24 hours
			IssuedAt:  time.Now().Unix(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token, nil

}

func verifySignature(msg string, signature string, owner common.Address, chainId int) (*common.Address, error) {

	sigHash := utils.GetEIP191SigHash(msg)
	sigBytes, err := hexutil.Decode(signature)
	if err != nil {
		return nil, err
	}
	if sigBytes[64] != 27 && sigBytes[64] != 28 {
		return nil, fmt.Errorf("Invalid signature recovery byte")
	}
	sigBytes[64] -= 27
	pubkey, err := crypto.SigToPub(sigHash.Bytes(), sigBytes)
	if err != nil {
		return nil, err
	}
	addr := crypto.PubkeyToAddress(*pubkey)
	// AS IN TEST CASES WALLET CANT BE A CONTRACT ADDRESS HENCE COMMENTED IT OUT
	// if addr != owner {
	// 	sigBytes[64] += 27
	// 	return utils.CheckERC1271Sig(sigHash, sigBytes, owner, chainId, a.config)
	// }
	return &addr, nil

}

func authenticate(ctx *gin.Context) {
	tokenString := ctx.GetHeader("Authorization")
	if tokenString == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization token"})
		ctx.Abort()
		return
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("invalid signing method")
		}

		return []byte("SECRET"), nil
	})

	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		ctx.Abort()
		return
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if userWallet, exists := claims["userWallet"]; exists {
			ctx.Set("userWallet", strings.ToLower(userWallet.(string)))
		} else {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			ctx.Abort()
			return
		}
	} else {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
		ctx.Abort()
		return
	}

	ctx.Next()
}

type CreateOrder struct {
	SendAddress          string `json:"sendAddress" binding:"required"`
	ReceiveAddress       string `json:"receiveAddress" binding:"required"`
	OrderPair            string `json:"orderPair" binding:"required"`
	SendAmount           string `json:"sendAmount" binding:"required"`
	ReceiveAmount        string `json:"receiveAmount" binding:"required"`
	SecretHash           string `json:"secretHash" binding:"required"`
	UserWalletBTCAddress string `json:"userWalletBTCAddress" binding:"required"`
}

var CurrentOrderID = 0

// mock handler
func (s *TestOrderBookServer) postOrders() gin.HandlerFunc {
	return func(c *gin.Context) {

		_, exists := c.Get("userWallet")
		Expect(exists).To(BeTrue())
		req := CreateOrder{}
		err := c.ShouldBindJSON(&req)
		Expect(err).To(BeNil())
		CurrentOrderID += 1

		c.JSON(http.StatusCreated, gin.H{
			"orderId": CurrentOrderID,
		})
	}
}
