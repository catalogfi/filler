package creator

import (
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	obStore "github.com/catalogfi/orderbook/store"
	"go.uber.org/zap"
)

type Creator interface {
	Start() error
	Stop()
}

type creator struct {
	btcAddress string
	ethAddress string
	restClient rest.Client
	strategy   Strategy
	store      store.Store
	logger     *zap.Logger
	quit       chan struct{}
	execWg     *sync.WaitGroup // Waitgroup to wait for all execution to finish
}

func NewCreator(
	btcAddress string,
	ethAddress string,
	restClient rest.Client,
	strategy Strategy,
	store store.Store,
	logger *zap.Logger,
) (Creator, error) {
	fromChain, toChain, _, _, err := model.ParseOrderPair(strategy.orderPair)
	if err != nil {
		return nil, err
	}

	if fromChain.IsBTC() {
		if err := obStore.CheckAddress(fromChain, btcAddress); err != nil {
			return nil, err
		}
		if err := obStore.CheckAddress(toChain, ethAddress); err != nil {
			return nil, err
		}
	} else {
		if err := obStore.CheckAddress(toChain, btcAddress); err != nil {
			return nil, err
		}
		if err := obStore.CheckAddress(fromChain, ethAddress); err != nil {
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
*/
func (c *creator) Stop() {
	close(c.quit)
	c.execWg.Wait()
}

func (c *creator) Start() error {

	expSetBack := time.Second
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

	timeInterval := int64(c.strategy.MaxTimeInterval) - int64(c.strategy.MinTimeInterval)
	if timeInterval < 0 {
		return fmt.Errorf("Invalid time interval Supplied: MinTimeInterval should be less than MaxTimeInterval")
	}

	// running Core logic in go-routine in order to make Start() function non-blocking
	c.execWg.Add(1)
	go func() {
		defer c.execWg.Done()

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

				randTimeInterval := rand.Int63n(timeInterval)
				if err != nil {
					c.logger.Error("failed generating random time interval", zap.Error(err))
					break
				}
				randTimeInterval += int64(c.strategy.MinTimeInterval)

				secret := [32]byte{}
				_, err = cryptoRand.Read(secret[:])
				if err != nil {
					c.logger.Error("failed generating random secret", zap.Error(err))
					break
				}
				secretHash := sha256.Sum256(secret[:])
				secretStr := hex.EncodeToString(secret[:])

				// TODO: virtual Balance Checks After InstantWallet
				id, err := c.restClient.CreateOrder(fromAddress, toAddress, c.strategy.orderPair, c.strategy.Amount.String(), receiveAmount.String(), hex.EncodeToString(secretHash[:]))
				if err != nil {
					c.logger.Error("failed creating order", zap.Error(err))
					break
				}

				if err := c.store.PutSecret(hex.EncodeToString(secretHash[:]), &secretStr, uint64(id)); err != nil {
					c.logger.Error("failed storing secret", zap.Error(err))
					break
				}

				select {
				case <-time.After(time.Duration(randTimeInterval) * time.Second):
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
