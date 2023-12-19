package creator

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"go.uber.org/zap"
)

type Creator interface {
	Start()
	Stop()
}

// add strategies
type creator struct {
	btcWallet  btcswap.Wallet
	ethWallet  ethswap.Wallet
	restClient rest.Client
	strategy   Strategy
	store      store.Store
	logger     *zap.Logger
	quit       chan struct{}
	execWg     *sync.WaitGroup
}

func NewCreator(
	btcWallet btcswap.Wallet,
	ethWallet ethswap.Wallet,
	restClient rest.Client,
	strategy Strategy,
	store store.Store,
	logger *zap.Logger,
	quit chan struct{},
) Creator {
	return &creator{
		btcWallet:  btcWallet,
		ethWallet:  ethWallet,
		restClient: restClient,
		strategy:   strategy,
		store:      store,
		logger:     logger,
		quit:       quit,
		execWg:     new(sync.WaitGroup),
	}
}

/*
- will gracefully stop all the creators
*/
func (c *creator) Stop() {
	defer func() {
		close(c.quit)

	}()
	c.quit <- struct{}{}
	c.execWg.Wait()
}

/*
- signer is the ethereum public address used to authenticate with
the orderbook server
- btcWallets and ethwallets respectively should be generated using
only one private, that is used by signer to create or fill the orders
*/
func (c *creator) Start() {
	defer c.execWg.Done()
	// to enable blocking stop message
	c.execWg.Add(1)

	// ctx, cancel := context.WithCancel(context.Background())
	expSetBack := time.Second

CONNECTIONLOOP:
	for {

		jwt, err := c.restClient.Login()
		if err != nil {
			c.logger.Error("failed logging in", zap.Error(err))
			return
		}

		c.restClient.SetJwt(jwt)

		fromChain, _, _, _, err := model.ParseOrderPair(c.strategy.orderPair)
		if err != nil {
			c.logger.Error("failed parsing order pair", zap.Error(err))
			return
		}

		fromAddress := c.ethWallet.Address().String()
		toAddress := c.btcWallet.Address().EncodeAddress()

		if fromChain.IsBTC() {
			fromAddress, toAddress = toAddress, fromAddress
		}

		c.logger.Info("Starting Auto Creator")
	CREATELOOP:
		for {

			randTimeInterval, err := rand.Int(rand.Reader, big.NewInt(int64(c.strategy.MaxTimeInterval-c.strategy.MinTimeInterval)))
			if err != nil {
				c.logger.Error("failed generating random time interval", zap.Error(err))
				break CREATELOOP
			}
			randTimeInterval.Add(randTimeInterval, big.NewInt(int64(c.strategy.MinTimeInterval)))

			receiveAmount := big.NewInt(c.strategy.Amount.Int64() * int64(10000-c.strategy.Fee) / 10000)

			secret := [32]byte{}
			_, err = rand.Read(secret[:])
			if err != nil {
				c.logger.Error("failed generating random secret", zap.Error(err))
				break CREATELOOP
			}
			secretHash := sha256.Sum256(secret[:])

			id, err := c.restClient.CreateOrder(fromAddress, toAddress, c.strategy.orderPair, c.strategy.Amount.String(), receiveAmount.String(), hex.EncodeToString(secretHash[:]))
			if err != nil {
				c.logger.Error("failed creating order", zap.Error(err))
				break CREATELOOP
			}

			secretStr := hex.EncodeToString(secret[:])

			if err := c.store.PutSecret(hex.EncodeToString(secretHash[:]), &secretStr, uint64(id)); err != nil {
				c.logger.Error("failed storing secret", zap.Error(err))
				break CREATELOOP
			}
			sleepTimer := time.NewTimer(time.Duration(randTimeInterval.Int64()) * time.Second)

			select {
			case <-sleepTimer.C:
				continue
			case <-c.quit:
				break CONNECTIONLOOP
			}
		}

		time.Sleep(expSetBack)
		if expSetBack < (8 * time.Second) {
			expSetBack *= 2
		}
	}
}
