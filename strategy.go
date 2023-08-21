package cobi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"go.uber.org/zap"
)

func RunAutoCreateStrategy(url string, keys utils.Keys, config model.Network, store store.Store, logger *zap.Logger, s AutoCreateStrategy, isIw bool) {
	defer logger.Info("exiting auto create strategy")

	signer, client, err := utils.LoadClient(url, keys, store, s.account, 0)
	if err != nil {
		logger.Error("failed to connect to the client", zap.Error(err))
		return
	}

	for {
		randTimeInterval, err := rand.Int(rand.Reader, big.NewInt(int64(s.MaxTimeInterval-s.MinTimeInterval)))
		if err != nil {
			logger.Error("can't create a random time", zap.Error(err))
			return
		}
		randTimeInterval.Add(randTimeInterval, big.NewInt(int64(s.MinTimeInterval)))

		randAmount, err := rand.Int(rand.Reader, new(big.Int).Sub(s.maxAmount, s.minAmount))
		if err != nil {
			logger.Error("can't create a random amount", zap.Error(err))
			return
		}
		randAmount.Add(randAmount, s.minAmount)

		fromChain, toChain, fromAsset, _, err := model.ParseOrderPair(s.orderPair)
		if err != nil {
			logger.Error("failed while parsing order pair", zap.Error(err))
			return
		}

		// Get the addresses on different chains.
		fromKey, err := keys.GetKey(fromChain, s.account, 0)
		if err != nil {
			logger.Error("failed while getting from key", zap.Error(err))
			return
		}
		fromAddress, err := fromKey.Address(fromChain)
		if err != nil {
			logger.Error("failed while getting address string", zap.Error(err))
			return
		}
		toKey, err := keys.GetKey(toChain, s.account, 0)
		if err != nil {
			logger.Error("failed while getting to key", zap.Error(err))
			return
		}
		toAddress, err := toKey.Address(toChain)
		if err != nil {
			logger.Error("failed while getting address string", zap.Error(err))
			return
		}

		balance, err := utils.VirtualBalance(fromChain, fromAddress, config, fromAsset, signer.Hex(), client, isIw)
		if err != nil {
			logger.Error("failed to get virtual balance", zap.String("address", fromAddress), zap.Error(err))
			return
		}

		if balance.Cmp(randAmount) < 0 {
			logger.Info("insufficient balance", zap.String("have", balance.String()), zap.String("need", randAmount.String()))
			continue
		}

		receiveAmount, err := s.priceStrategy.CalculatereceiveAmount(randAmount)
		if err != nil {
			logger.Error("failed while getting address string", zap.Error(err))
			return
		}

		secret := [32]byte{}
		_, err = rand.Read(secret[:])
		if err != nil {
			logger.Error("failed to read secret", zap.Error(err))
			return
		}
		secretHash := sha256.Sum256(secret[:])

		id, err := client.CreateOrder(fromAddress, toAddress, s.orderPair, randAmount.String(), receiveAmount.String(), hex.EncodeToString(secretHash[:]))
		if err != nil {
			logger.Error("failed while creating order", zap.Error(err))
			return
		}

		if err := store.UserStore(s.account).PutSecret(hex.EncodeToString(secretHash[:]), hex.EncodeToString(secret[:]), uint64(id)); err != nil {
			logger.Error("failed to store secret", zap.Error(err))
			return
		}

		time.Sleep(time.Duration(randTimeInterval.Int64()) * time.Second)
	}
}

func RunAutoFillStrategy(url string, keys utils.Keys, config model.Network, store store.Store, logger *zap.Logger, s AutoFillStrategy, isIw bool) {
	defer logger.Info("exiting auto fill strategy")

	// Load keys
	signer, client, err := utils.LoadClient(url, keys, store, s.account, 0)
	if err != nil {
		logger.Error("can't load the client", zap.Error(err))
		return
	}

	for {
		price, err := s.PriceStrategy().Price()
		if err != nil {
			logger.Error("failed calculating price", zap.Error(err))
			continue
		}

		orders, err := client.GetOrders(rest.GetOrdersFilter{
			Maker:     strings.Join(s.Makers, ","),
			OrderPair: s.OrderPair(),
			MinPrice:  price,
			MaxPrice:  math.MaxFloat64,
			Status:    int(model.Created),
			Verbose:   true,
		})
		if err != nil {
			logger.Error("failed parsing order pair", zap.Error(err), zap.Any("filter", rest.GetOrdersFilter{
				Maker:     strings.Join(s.Makers, ","),
				OrderPair: s.OrderPair(),
				MinPrice:  price,
				MaxPrice:  math.MaxFloat64,
				Status:    int(model.Created),
				Verbose:   true,
			}))
			continue
		}

		for _, order := range orders {
			toChain, fromChain, _, toAsset, err := model.ParseOrderPair(order.OrderPair)
			if err != nil {
				logger.Error("failed parsing order pair", zap.Error(err))
				return
			}

			// Get the addresses on different chains.
			fromKey, err := keys.GetKey(fromChain, s.account, 0)
			if err != nil {
				logger.Error("failed getting from key", zap.Error(err))
				return
			}
			fromAddress, err := fromKey.Address(fromChain)
			if err != nil {
				logger.Error("failed getting from address string", zap.Error(err))
				return
			}
			toKey, err := keys.GetKey(toChain, s.account, 0)
			if err != nil {
				logger.Error("failed getting to key", zap.Error(err))
				return
			}
			toAddress, err := toKey.Address(toChain)
			if err != nil {
				logger.Error("failed getting to address string", zap.Error(err))
				return
			}

			balance, err := utils.VirtualBalance(toChain, toAddress, config, toAsset, signer.Hex(), client, isIw)
			if err != nil {
				logger.Error("failed to get virtual balance", zap.String("address", toAddress), zap.Error(err))
				continue
			}

			if order.FollowerAtomicSwap == nil {
				logger.Error("malformed order", zap.Any("order", order))
				continue
			}

			orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
			if !ok {
				logger.Info("failed to get order amount", zap.Error(err))
				continue
			}

			if balance.Cmp(orderAmount) < 0 {
				logger.Info("insufficient balance", zap.String("have", balance.String()), zap.String("need", orderAmount.String()))
				continue
			}

			if err := client.FillOrder(order.ID, fromAddress, toAddress); err != nil {
				logger.Error("failed to fill the order ❌", zap.Uint("id", order.ID), zap.Error(err))
				continue
			}

			if err = store.UserStore(s.account).PutSecretHash(order.SecretHash, uint64(order.ID)); err != nil {
				logger.Info("failed storing secret hash: %v", zap.Error(err))
				continue
			}
			logger.Info("filled order ✅", zap.Uint("id", order.ID))
		}
		time.Sleep(10 * time.Second)
	}
}
