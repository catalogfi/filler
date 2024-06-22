package creator

import (
	"context"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
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
	btcWallet  btcswap.Wallet
	ethWallets map[model.Chain]ethswap.Wallet
	stratagies []Strategy
	restClient rest.Client
	store      Store
	logger     *zap.Logger
	quit       chan struct{}
	execWg     *sync.WaitGroup
}

func New(
	stratagies []Strategy,
	btcWallet btcswap.Wallet,
	ethWallets map[model.Chain]ethswap.Wallet,
	restClient rest.Client,
	store Store,
	logger *zap.Logger,
) Creator {
	return &creator{
		btcWallet:  btcWallet,
		ethWallets: ethWallets,
		restClient: restClient,
		stratagies: stratagies,
		store:      store,
		logger:     logger,
		quit:       make(chan struct{}),
		execWg:     new(sync.WaitGroup),
	}
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
	for _, strategy := range c.stratagies {
		go func(s Strategy) {
			if err := c.create(s); err != nil {
				c.logger.Error("create strategy failed", zap.Error(err))
			}
		}(strategy)
	}
	return nil
}

func (c *creator) create(s Strategy) error {
	// Get addresses for sender and receiver
	fromChain, toChain, _, toAsset, err := model.ParseOrderPair(s.OrderPair)
	if err != nil {
		return err
	}
	fromAddress := c.ethWallets[fromChain].Address().Hex()
	toAddress := c.btcWallet.Address().EncodeAddress()
	if fromChain.IsBTC() {
		fromAddress, toAddress = toAddress, fromAddress
	}

	receiveAmount := big.NewInt(s.Amount.Int64() * int64(10000-s.Fee) / 10000)

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
			interval := 30 * time.Second

			if err := c.balanceCheck(fromChain, toChain, toAsset, s.Amount, interval); err != nil {
				c.logger.Debug("balance check", zap.Error(err))
				continue
			}

			_, err = c.restClient.CreateOrder(fromAddress, toAddress, s.OrderPair, s.Amount.String(), receiveAmount.String(), hex.EncodeToString(secretHash[:]))
			if err != nil {
				c.logger.Error("failed creating order", zap.Error(err))
				break
			}

			if err := c.store.PutSecret(secretHash[:], secret[:]); err != nil {
				c.logger.Error("failed storing secret", zap.Error(err))
				break
			}

			select {
			case <-time.After(s.TimeInterval()):
				continue
			case <-c.quit:
				c.logger.Info("received quit channel signal")
				return nil
			}
		}

		time.Sleep(expSetBack)
		if expSetBack < (8 * time.Second) {
			expSetBack *= 2
		}
	}
}

func (f *creator) balanceCheck(from, to model.Chain, asset model.Asset, amount *big.Int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Check the `from` chain first, we only need to make sure we have enough eth to pay the gas if it's a evm chain
	if from.IsEVM() {
		ethWallet := f.ethWallets[from]
		ethBalance, err := ethWallet.Balance(ctx, true)
		if err != nil {
			return fmt.Errorf("failed to get eth balance, %v", err)
		}
		if ethBalance.Cmp(big.NewInt(1e16)) <= 0 {
			return fmt.Errorf("insufficent ETH on %v", from)
		}
	}

	// Check the `to` chain, we'll need to make sure
	// 1) enough gas if it's an evm chain
	// 2) enough btc/wbtc to execute the order, this including pending orders which haven't been executed
	if to.IsBTC() {
		// Check if the balance is enough
		balance, err := f.btcWallet.Balance(ctx)
		if err != nil {
			return err
		}
		unexecuted, err := f.unexecutedAmount(to, asset)
		if err != nil {
			return err
		}
		if balance < unexecuted.Int64()+amount.Int64() {
			return fmt.Errorf("%v balance is not enough, required = %v, has = %v unexecuted =%v", to, unexecuted.Int64()+amount.Int64(), balance, unexecuted.String())
		}
	} else {
		wallet := f.ethWallets[to]

		// Check if the balance is enough
		balance, err := wallet.TokenBalance(ctx, true)
		if err != nil {
			return fmt.Errorf("failed to get token balance, %v", err)
		}
		unexecuted, err := f.unexecutedAmount(to, asset)
		if err != nil {
			return err
		}
		required := unexecuted.Add(unexecuted, amount)
		if balance.Cmp(required) <= 0 {
			return fmt.Errorf("%v balance is not enough, required = %v, has = %v, unexecuted =%v", to, required.String(), balance.String(), unexecuted.String())
		}
	}
	return nil
}

func (f *creator) unexecutedAmount(chain model.Chain, asset model.Asset) (*big.Int, error) {
	filter := rest.GetOrdersFilter{
		Taker:   f.ethWallets[chain].Address().Hex(),
		Verbose: true,
		Status:  int(model.Filled),
	}
	orders, err := f.restClient.GetOrders(filter)
	if err != nil {
		return nil, err
	}
	amount := big.NewInt(0)
	for _, order := range orders {
		if order.FollowerAtomicSwap.Chain == chain &&
			order.FollowerAtomicSwap.Asset == asset &&
			order.FollowerAtomicSwap.Status == model.NotStarted {
			orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
			if !ok {
				return nil, err
			}
			amount.Add(amount, orderAmount)
		}
	}
	return amount, nil
}

func (f *creator) addr(chain model.Chain) string {
	if chain.IsBTC() {
		return f.btcWallet.Address().EncodeAddress()
	} else {
		return f.ethWallets[chain].Address().Hex()
	}
}
