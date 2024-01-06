package creator

import (
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/util"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"go.uber.org/zap"
)

type Store interface {
	// PutSecret stores the secret.
	PutSecret(hash, secret []byte) error
}

type Creator interface {
	Start() error
	Stop()
}

type creator struct {
	btcAddress string
	ethAddress string
	restClient rest.Client
	strategy   *Strategy
	store      Store
	logger     *zap.Logger
	quit       chan struct{}
	execWg     *sync.WaitGroup
}

func NewCreator(
	btcAddress string,
	ethAddress string,
	restClient rest.Client,
	strategy *Strategy,
	store Store,
	logger *zap.Logger,
) (Creator, error) {
	fromChain, toChain, _, _, err := model.ParseOrderPair(strategy.orderPair)
	if err != nil {
		return nil, err
	}

	if fromChain.IsBTC() {
		if err := util.ValidateAddress(fromChain, btcAddress); err != nil {
			return nil, err
		}
		if err := util.ValidateAddress(toChain, ethAddress); err != nil {
			return nil, err
		}
	} else {
		if err := util.ValidateAddress(toChain, btcAddress); err != nil {
			return nil, err
		}
		if err := util.ValidateAddress(fromChain, ethAddress); err != nil {
			return nil, err
		}
	}

	return &creator{
		btcAddress: btcAddress,
		ethAddress: ethAddress,
		restClient: restClient,
		strategy:   strategy,
		store:      store,
		logger:     logger,
		quit:       make(chan struct{}),
		execWg:     new(sync.WaitGroup),
	}, nil
}

/*
- will gracefully stop all the creators
- This cannot be called multiple times.
*/
func (c *creator) Stop() {
	close(c.quit)
	c.execWg.Wait()
}

func (c *creator) Start() error {
	// Get addresses for sender and receiver
	fromChain, _, _, _, err := model.ParseOrderPair(c.strategy.orderPair)
	if err != nil {
		return err
	}
	fromAddress := c.ethAddress
	toAddress := c.btcAddress
	if fromChain.IsBTC() {
		fromAddress, toAddress = toAddress, fromAddress
	}

	receiveAmount := big.NewInt(c.strategy.Amount.Int64() * int64(10000-c.strategy.Fee) / 10000)

	// running Core logic in go-routine in order to make Start() function non-blocking
	c.execWg.Add(1)
	go func() {
		defer c.execWg.Done()

		expSetBack := time.Second
		for {

			// If JWT expires, login again
			jwt, err := c.restClient.Login()
			if err != nil {
				c.logger.Error("failed logging in", zap.Error(err))
				time.Sleep(expSetBack) // Wait for expSetBack before retrying
				continue
			}
			if err := c.restClient.SetJwt(jwt); err != nil {
				c.logger.Error("failed setting jwt", zap.Error(err))
				continue
			}

			c.logger.Info("Starting Auto Creator")

			for {
				secret := [32]byte{}
				_, err = cryptoRand.Read(secret[:])
				if err != nil {
					c.logger.Error("failed generating random secret", zap.Error(err))
					break
				}
				secretHash := sha256.Sum256(secret[:])

				// TODO: virtual Balance Checks After InstantWallet
				_, err = c.restClient.CreateOrder(fromAddress, toAddress, c.strategy.orderPair, c.strategy.Amount.String(), receiveAmount.String(), hex.EncodeToString(secretHash[:]))
				if err != nil {
					c.logger.Error("failed creating order", zap.Error(err))
					break
				}

				if err := c.store.PutSecret(secretHash[:], secret[:]); err != nil {
					c.logger.Error("failed storing secret", zap.Error(err))
					break
				}

				select {
				case <-time.After(c.strategy.TimeInterval()):
					continue
				case <-c.quit:
					c.logger.Info("received quit channel signal")
					return
				}
			}

			time.Sleep(expSetBack)
			if expSetBack < (8 * time.Second) {
				expSetBack *= 2
			}
		}
	}()

	return nil
}
