package strategy

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
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
	UseIw         bool            `json:"UseIw"`
}

type AutoCreateStrategy struct {
	MinTimeInterval uint32
	MaxTimeInterval uint32

	account       uint32
	minAmount     *big.Int
	maxAmount     *big.Int
	orderPair     string
	priceStrategy PriceStrategy
	UseIw         bool
}

type AutoFillStrategy struct {
	Makers []string

	account       uint32
	minAmount     *big.Int
	maxAmount     *big.Int
	orderPair     string
	priceStrategy PriceStrategy
	UseIw         bool
}

func StrategyToAutoCreateStrategy(strategy Strategy) (AutoCreateStrategy, error) {
	autoCreateStrategy := AutoCreateStrategy{
		MinTimeInterval: strategy.MinTimeInterval,
		MaxTimeInterval: strategy.MaxTimeInterval,
		account:         strategy.Account,
		minAmount:       nil, // need to parse and convert the string to big.Int
		maxAmount:       nil, // need to parse and convert the string to big.Int
		orderPair:       strategy.OrderPair,
		priceStrategy:   nil, // need to convert strategy.PriceStrategy to PriceStrategy
		UseIw:           strategy.UseIw,
	}

	// Parse and convert MinAmount and MaxAmount
	minAmount, ok := new(big.Int).SetString(strategy.MinAmount, 10)
	if !ok {
		return AutoCreateStrategy{}, fmt.Errorf("failed to convert minAmount to big.Int")
	}
	autoCreateStrategy.minAmount = minAmount

	maxAmount, ok := new(big.Int).SetString(strategy.MaxAmount, 10)
	if !ok {
		return AutoCreateStrategy{}, fmt.Errorf("failed to convert maxAmount to big.Int")
	}
	autoCreateStrategy.maxAmount = maxAmount

	// Parse and convert PriceStrategy
	types := strings.Split(strategy.StrategyType, "-")

	priceStrategy, err := UnmarshalPriceStrategy(types[0], strategy.PriceStrategy)
	if err != nil {
		return AutoCreateStrategy{}, fmt.Errorf("failed to convert PriceStrategy: %v", err)
	}
	autoCreateStrategy.priceStrategy = priceStrategy

	return autoCreateStrategy, nil
}
func StrategyToAutoFillStrategy(strategy Strategy) (AutoFillStrategy, error) {
	autoFillStrategy := AutoFillStrategy{
		Makers:        strategy.Makers,
		account:       strategy.Account,
		minAmount:     nil, // need to parse and convert the string to big.Int
		maxAmount:     nil, // need to parse and convert the string to big.Int
		orderPair:     strategy.OrderPair,
		priceStrategy: nil, // need to convert strategy.PriceStrategy to PriceStrategy
		UseIw:         strategy.UseIw,
	}

	// Parse and convert MinAmount and MaxAmount
	minAmount, ok := new(big.Int).SetString(strategy.MinAmount, 10)
	if !ok {
		return AutoFillStrategy{}, fmt.Errorf("failed to convert minAmount to big.Int")
	}
	autoFillStrategy.minAmount = minAmount

	maxAmount, ok := new(big.Int).SetString(strategy.MaxAmount, 10)
	if !ok {
		return AutoFillStrategy{}, fmt.Errorf("failed to convert maxAmount to big.Int")
	}
	autoFillStrategy.maxAmount = maxAmount

	// Parse and convert PriceStrategy
	types := strings.Split(strategy.StrategyType, "-")

	priceStrategy, err := UnmarshalPriceStrategy(types[0], strategy.PriceStrategy)
	if err != nil {
		return AutoFillStrategy{}, fmt.Errorf("failed to convert PriceStrategy: %v", err)
	}
	autoFillStrategy.priceStrategy = priceStrategy

	return autoFillStrategy, nil
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

func UnmarshalStrategy(data []byte) ([]Strategy, error) {
	strategies := []Strategy{}
	if err := json.Unmarshal(data, &strategies); err != nil {
		return nil, err
	}
	return strategies, nil
}
func UnmarshalStrategyToAuto(data []byte) ([]AutoStrategy, error) {
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
