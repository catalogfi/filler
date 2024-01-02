package filler_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest/utils"
	"github.com/dgrijalva/jwt-go"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spruceid/siwe-go"
	"go.uber.org/zap"
)

func TestFiller(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filler Suite")
}

var (
	server *TestOrderBookServer
	Cancel context.CancelFunc
)

var _ = BeforeSuite(func() {
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

	s.router.GET("/", s.socket())
	s.router.GET("/nonce", s.nonce())
	s.router.POST("/verify", s.verify())

	authRoutes := s.router.Group("/")
	authRoutes.Use(authenticate)
	{
		authRoutes.PUT("/orders/:id", s.fillOrder())
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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (s *TestOrderBookServer) socket() gin.HandlerFunc {
	return func(c *gin.Context) {
		mx := new(sync.RWMutex)
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to upgrade to websocket %v", err)})
			return
		}
		defer func() {
			ws.Close()
		}()

		for resp := range s.Msg {
			mx.Lock()
			err = ws.WriteJSON(map[string]interface{}{
				"type": fmt.Sprintf("%T", resp),
				"msg":  resp,
			})
			mx.Unlock()
			if err != nil {
				s.logger.Debug("failed to write message", zap.Error(err))
				return
			}
		}

	}
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

func (s *TestOrderBookServer) fillOrder() gin.HandlerFunc {
	return func(c *gin.Context) {
		// mock Handler
		orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
		Expect(err).To(BeNil())
		Expect(orderID).To(BeNumerically(">", 0))
		filler, exists := c.Get("userWallet")
		Expect(exists).To(BeTrue())
		// check if filler is an etheruem address
		Expect(len(filler.(string))).To(Equal(42))
		c.JSON(http.StatusAccepted, gin.H{})
	}
}
