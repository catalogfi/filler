package cobi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type Strategy struct {
	StrategyType string `json:"strategyType"`

	// Create
	MinTimeInterval uint32 `json:"minTimeInterval"`
	MaxTimeInterval uint32 `json:"maxTimeInterval"`

	// Fill
	Makers []string `json:"makers"`

	// Common
	Account       uint32          `json:"account"`
	MinAmount     string          `json:"minAmount"`
	MaxAmount     string          `json:"maxAmount"`
	OrderPair     string          `json:"orderPair"`
	PriceStrategy json.RawMessage `json:"priceStrategy"`
}

func Start(url string, entropy, strategy []byte, config model.Config, store Store, logger *zap.Logger) error {
	wg := new(sync.WaitGroup)
	activeAccounts := map[uint32]bool{}
	strategies, err := UnmarshalStrategy(strategy)
	if err != nil {
		logger.Error("failed to unmarshal strategy", zap.Error(err))
		return err
	}
	for index, strategy := range strategies {
		if !activeAccounts[strategy.Account()] {
			wg.Add(1)
			go func(account uint32, logger *zap.Logger) {
				defer wg.Done()
				RunExecute(entropy, account, url, store, config, logger)
			}(strategy.Account(), logger.With(zap.Uint32("executor", strategy.Account())))
			activeAccounts[strategy.Account()] = true
		}

		childLogger := logger.With(zap.String("strategy", fmt.Sprintf("%T", strategy)), zap.String("orderPair", strategy.OrderPair()), zap.Uint32("account", strategy.Account()))
		wg.Add(1)
		go func(i int, logger *zap.Logger) {
			defer wg.Done()
			switch strategy := strategies[i].(type) {
			case AutoFillStrategy:
				err := RunAutoFillStrategy(url, entropy, config, store, logger, strategy)
				logger.Error("auto fill strategy ended with", zap.Error(err))
			case AutoCreateStrategy:
				err := RunAutoCreateStrategy(url, entropy, config, store, logger, strategy)
				logger.Error("auto create strategy ended with", zap.Error(err))
			default:
				logger.Error("unexpected strategy")
			}
		}(index, childLogger)
	}
	wg.Wait()
	return nil
}

func RunAutoCreateStrategy(url string, entropy []byte, config model.Config, store Store, logger *zap.Logger, s AutoCreateStrategy) error {
	defer logger.Info("exiting auto create strategy", zap.String("orderPair", s.orderPair), zap.String("priceStrategy", fmt.Sprintf("%T", s.priceStrategy)))

	// Load keys
	keys := NewKeys()
	key, err := keys.GetKey(entropy, model.Ethereum, s.account, 0)
	if err != nil {
		return fmt.Errorf("Error while getting the signing key: %v", err)
	}
	privKey, err := key.ECDSA()
	if err != nil {
		cobra.CheckErr(err)
	}

	client := rest.NewClient(fmt.Sprintf("http://%s", url), privKey.D.Text(16))
	maker := crypto.PubkeyToAddress(privKey.PublicKey)

	token, err := client.Login()
	if err != nil {
		return fmt.Errorf("Error while getting the signing key: %v", err)
	}
	if err := client.SetJwt(token); err != nil {
		return fmt.Errorf("Error to parse signing key: %v", err)
	}
	for {
		randTimeInterval, err := rand.Int(rand.Reader, big.NewInt(int64(s.MaxTimeInterval-s.MinTimeInterval)))
		if err != nil {
			return fmt.Errorf("can't create a random value: %v", err)
		}
		randTimeInterval.Add(randTimeInterval, big.NewInt(int64(s.MinTimeInterval)))

		randAmount, err := rand.Int(rand.Reader, new(big.Int).Sub(s.maxAmount, s.minAmount))
		if err != nil {
			return fmt.Errorf("can't create a random value: %v", err)
		}
		randAmount.Add(randAmount, s.minAmount)

		fromChain, toChain, fromAsset, _, err := model.ParseOrderPair(s.orderPair)
		if err != nil {
			return fmt.Errorf("failed while parsing order pair: %v", err)
		}

		// Get the addresses on different chains.
		fromKey, err := keys.GetKey(entropy, fromChain, s.account, 0)
		if err != nil {
			return fmt.Errorf("failed while getting from key: %v", err)
		}
		fromAddress, err := fromKey.Address(fromChain)
		if err != nil {
			return fmt.Errorf("failed while getting address string: %v", err)
		}
		toKey, err := keys.GetKey(entropy, fromChain, s.account, 0)
		if err != nil {
			return fmt.Errorf("failed while getting to key: %v", err)
		}
		toAddress, err := toKey.Address(toChain)
		if err != nil {
			return fmt.Errorf("failed while getting address string: %v", err)
		}

		balance, err := getVirtualBalance(fromChain, fromAsset, s.orderPair, config, client, maker.Hex(), fromAddress, false)
		if err != nil {
			logger.Info("failed to get virtual balance", zap.String("address", fromAddress), zap.Error(err))
			continue
		}

		if balance.Cmp(randAmount) < 0 {
			logger.Info("insufficient balance", zap.String("have", balance.String()), zap.String("need", randAmount.String()))
			continue
		}

		receiveAmount, err := s.priceStrategy.CalculatereceiveAmount(randAmount)
		if err != nil {
			return fmt.Errorf("failed while getting address string: %v", err)
		}

		secret := [32]byte{}
		_, err = rand.Read(secret[:])
		if err != nil {
			return fmt.Errorf("failed to read secret: %v", err)
		}
		secretHash := sha256.Sum256(secret[:])

		id, err := client.CreateOrder(fromAddress, toAddress, s.orderPair, randAmount.String(), receiveAmount.String(), hex.EncodeToString(secretHash[:]))
		if err != nil {
			return fmt.Errorf("failed while creating order: %v", err)
		}

		if err := store.PutSecret(s.account, hex.EncodeToString(secretHash[:]), hex.EncodeToString(secret[:]), uint64(id)); err != nil {
			return fmt.Errorf("failed to store secret: %v", err)
		}

		time.Sleep(time.Duration(randTimeInterval.Int64()) * time.Second)
	}
}

func RunAutoFillStrategy(url string, entropy []byte, config model.Config, store Store, logger *zap.Logger, s AutoFillStrategy) error {
	defer logger.Info("exiting auto fill strategy", zap.String("orderPair", s.orderPair), zap.String("priceStrategy", fmt.Sprintf("%T", s.priceStrategy)))

	// Load keys
	keys := NewKeys()
	key, err := keys.GetKey(entropy, model.Ethereum, s.account, 0)
	if err != nil {
		return fmt.Errorf("failed to get the signing key: %v", err)
	}
	privKey, err := key.ECDSA()
	if err != nil {
		return fmt.Errorf("failed to get the ecdsa key: %v", err)
	}
	taker := crypto.PubkeyToAddress(privKey.PublicKey)
	client := rest.NewClient(fmt.Sprintf("http://%s", url), privKey.D.Text(16))

	for {
		price, err := s.PriceStrategy().Price()
		if err != nil {
			logger.Info("failed calculating price", zap.Error(err))
			continue
		}

		orders, err := client.GetOrders(rest.GetOrdersFilter{
			Maker:     strings.Join(s.Makers, ","),
			OrderPair: s.OrderPair(),
			MinPrice:  price,
			MaxPrice:  math.MaxFloat64,
			Status:    int(model.OrderCreated),
			Verbose:   true,
		})
		if err != nil {
			logger.Info("failed parsing order pair", zap.Error(err), zap.Any("filter", rest.GetOrdersFilter{
				Maker:     strings.Join(s.Makers, ","),
				OrderPair: s.OrderPair(),
				MinPrice:  price,
				MaxPrice:  math.MaxFloat64,
				Status:    int(model.OrderCreated),
			}))
			continue
		}

		for _, order := range orders {
			toChain, fromChain, _, toAsset, err := model.ParseOrderPair(order.OrderPair)
			if err != nil {
				return fmt.Errorf("failed parsing order pair: %v", err)
			}

			// Get the addresses on different chains.
			fromKey, err := keys.GetKey(entropy, fromChain, s.account, 0)
			if err != nil {
				return fmt.Errorf("failed getting from key: %v", err)
			}
			fromAddress, err := fromKey.Address(fromChain)
			if err != nil {
				return fmt.Errorf("failed getting address string: %v", err)
			}
			toKey, err := keys.GetKey(entropy, toChain, s.account, 0)
			if err != nil {
				return fmt.Errorf("failed getting to key: %v", err)
			}
			toAddress, err := toKey.Address(toChain)
			if err != nil {
				return fmt.Errorf("failed getting address string: %v", err)
			}

			balance, err := getVirtualBalance(toChain, toAsset, s.OrderPair(), config, client, taker.Hex(), toAddress, true)
			if err != nil {
				logger.Info("failed to get virtual balance", zap.String("address", toAddress), zap.Error(err))
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
				logger.Info("failed to fill the order ❌", zap.Uint("id", order.ID), zap.Error(err))
				continue
			}

			if err = store.PutSecretHash(s.account, order.SecretHash, uint64(order.ID)); err != nil {
				logger.Info("failed storing secret hash: %v", zap.Error(err))
				continue
			}
			logger.Info("filled order ✅", zap.Uint("id", order.ID))
		}
		time.Sleep(10 * time.Second)
	}
}

type AutoCreateStrategy struct {
	MinTimeInterval uint32
	MaxTimeInterval uint32

	account       uint32
	minAmount     *big.Int
	maxAmount     *big.Int
	orderPair     string
	priceStrategy PriceStrategy
}

type AutoFillStrategy struct {
	Makers []string

	account       uint32
	minAmount     *big.Int
	maxAmount     *big.Int
	orderPair     string
	priceStrategy PriceStrategy
}

type AutoStrategy interface {
	MinAmount() *big.Int
	MaxAmount() *big.Int
	OrderPair() string
	Account() uint32
	PriceStrategy() PriceStrategy
}

func (s AutoFillStrategy) MinAmount() *big.Int {
	return s.minAmount
}
func (s AutoFillStrategy) MaxAmount() *big.Int {
	return s.maxAmount
}
func (s AutoFillStrategy) OrderPair() string {
	return s.orderPair
}
func (s AutoFillStrategy) PriceStrategy() PriceStrategy {
	return s.priceStrategy
}
func (s AutoFillStrategy) Account() uint32 {
	return s.account
}

func (s AutoCreateStrategy) MinAmount() *big.Int {
	return s.minAmount
}
func (s AutoCreateStrategy) MaxAmount() *big.Int {
	return s.maxAmount
}
func (s AutoCreateStrategy) OrderPair() string {
	return s.orderPair
}
func (s AutoCreateStrategy) PriceStrategy() PriceStrategy {
	return s.priceStrategy
}
func (s AutoCreateStrategy) Account() uint32 {
	return s.account
}

func UnmarshalStrategy(data []byte) ([]AutoStrategy, error) {
	strategies := []Strategy{}
	if err := json.Unmarshal(data, &strategies); err != nil {
		return nil, err
	}

	autoStrategies := make([]AutoStrategy, len(strategies))
	for i, strategy := range strategies {
		types := strings.Split(strategy.StrategyType, "-")
		if len(types) != 2 {
			return nil, fmt.Errorf("invalid strategy type: %v", strategy.StrategyType)
		}

		priceStrategy, err := UnmarshalPriceStrategy(types[0], strategy.PriceStrategy)
		if err != nil {
			return nil, err
		}
		minAmount, ok := new(big.Int).SetString(strategy.MinAmount, 10)
		if !ok {
			return nil, fmt.Errorf("failed to decode min amount: %v", err)
		}
		maxAmount, ok := new(big.Int).SetString(strategy.MaxAmount, 10)
		if !ok {
			return nil, fmt.Errorf("failed to decode max amount: %v", err)
		}

		switch strings.ToLower(types[1]) {
		case "fill":
			autoStrategies[i] = AutoFillStrategy{
				Makers:        strategy.Makers,
				account:       strategy.Account,
				minAmount:     minAmount,
				maxAmount:     maxAmount,
				orderPair:     strategy.OrderPair,
				priceStrategy: priceStrategy,
			}
		case "create":
			autoStrategies[i] = AutoCreateStrategy{
				account:         strategy.Account,
				MinTimeInterval: strategy.MinTimeInterval,
				MaxTimeInterval: strategy.MaxTimeInterval,
				minAmount:       minAmount,
				maxAmount:       maxAmount,
				orderPair:       strategy.OrderPair,
				priceStrategy:   priceStrategy,
			}
		default:
			return nil, fmt.Errorf("unknown auto strategy: %v", err)
		}
	}

	return autoStrategies, nil
}

func MarshalStrategy(autoStrategies []AutoStrategy) ([]byte, error) {
	strategies := make([]Strategy, len(autoStrategies))
	for i, autoStrategy := range autoStrategies {
		priceType, priceData, err := MarshalPriceStrategy(autoStrategy.PriceStrategy())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal price strategy: %v", err)
		}
		strategies[i] = Strategy{
			OrderPair:     autoStrategy.OrderPair(),
			MinAmount:     autoStrategy.MinAmount().String(),
			MaxAmount:     autoStrategy.MaxAmount().String(),
			PriceStrategy: priceData,
			Account:       autoStrategy.Account(),
		}
		switch autoStrategy := autoStrategy.(type) {
		case AutoCreateStrategy:
			strategies[i].StrategyType = strings.Join([]string{priceType, "create"}, "-")
			strategies[i].MinTimeInterval = autoStrategy.MinTimeInterval
			strategies[i].MaxTimeInterval = autoStrategy.MaxTimeInterval
		case AutoFillStrategy:
			strategies[i].StrategyType = strings.Join([]string{priceType, "fill"}, "-")
			strategies[i].Makers = autoStrategy.Makers
		default:
			return nil, fmt.Errorf("unexpected auto strategy type: %T", autoStrategy)
		}
	}
	return json.Marshal(strategies)
}

type PriceStrategy interface {
	Price() (float64, error)
	CalculatereceiveAmount(val *big.Int) (*big.Int, error)
}

func MarshalPriceStrategy(strategy PriceStrategy) (string, json.RawMessage, error) {
	switch strategy := strategy.(type) {
	case Likewise:
		strategyBytes, err := json.Marshal(strategy)
		if err != nil {
			return "", nil, err
		}
		return "likewise", strategyBytes, nil
	default:
		return "", nil, fmt.Errorf("unknown price strategy")
	}
}

func UnmarshalPriceStrategy(priceStrategy string, data []byte) (PriceStrategy, error) {
	switch priceStrategy {
	case "likewise":
		lw := Likewise{}
		if err := json.Unmarshal(data, &lw); err != nil {
			return nil, err
		}
		return lw, nil
	default:
		return nil, fmt.Errorf("unknown strategy")
	}
}

type Likewise struct {
	Fee uint64 `json:"fee"`
}

// FEE is in BIPS, 1 BIP = 0.01% and 10000 BIPS = 100%
func (lw Likewise) CalculatereceiveAmount(val *big.Int) (*big.Int, error) {
	return big.NewInt(val.Int64() * int64(10000-lw.Fee) / 10000), nil
}

func (lw Likewise) Price() (float64, error) {
	return float64(10000) / float64(10000-lw.Fee), nil
}

func NewLikewise(feeInBips uint64) (PriceStrategy, error) {
	return &Likewise{}, nil
}

func getVirtualBalance(chain model.Chain, asset model.Asset, op string, config model.Config, client rest.Client, signer, address string, isFill bool) (*big.Int, error) {
	balance, err := getBalance(chain, address, config, asset)
	if err != nil {
		return nil, err
	}

	fillOrders, err := client.GetOrders(rest.GetOrdersFilter{
		Taker:     signer,
		OrderPair: op,
		Status:    int(model.OrderFilled),
		Verbose:   true,
	})
	if err != nil {
		return nil, err
	}
	createOrders, err := client.GetOrders(rest.GetOrdersFilter{
		Maker:     signer,
		OrderPair: op,
		Status:    int(model.OrderCreated),
		Verbose:   true,
	})
	if err != nil {
		return nil, err
	}
	orders := append(fillOrders, createOrders...)

	commitedAmount := big.NewInt(0)
	for _, order := range orders {
		orderAmt, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
		if !ok {
			return nil, err
		}
		commitedAmount.Add(commitedAmount, orderAmt)
	}

	return new(big.Int).Sub(balance, commitedAmount), nil
}
