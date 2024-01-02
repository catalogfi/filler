package creator

import (
	"math/big"
	"math/rand"
	"time"
)

type Strategy struct {
	MinTimeInterval uint32 // minimum time interval in seconds to wait for Next Order Creation
	MaxTimeInterval uint32 // maximum time interval in seconds to wait for Next Order Creation
	Amount          *big.Int
	orderPair       string
	Fee             float64 // fee(bips) converted to Fee
}

func NewStrategy(minTimeInterval uint32, maxTimeInterval uint32, Amount *big.Int, orderPair string, Fee float64) *Strategy {
	if minTimeInterval > maxTimeInterval {
		panic("Invalid time interval Supplied: MinTimeInterval should be less than MaxTimeInterval")
	}
	return &Strategy{
		MinTimeInterval: minTimeInterval,
		MaxTimeInterval: maxTimeInterval,
		Amount:          Amount,
		orderPair:       orderPair,
		Fee:             Fee,
	}
}

func StrategyWithDefaults(orderPair string) *Strategy {
	Fee := 10
	return &Strategy{
		MinTimeInterval: 10,
		MaxTimeInterval: 100,
		Amount:          big.NewInt(1e7),
		orderPair:       orderPair,
		Fee:             float64(10000) / float64(10000-Fee), // fee's should be converted to price (fee is in bips)
	}
}

func (strategy *Strategy) TimeInterval() time.Duration {
	timeInterval := int64(strategy.MaxTimeInterval) - int64(strategy.MinTimeInterval)
	randTimeInterval := rand.Int63n(timeInterval)
	randTimeInterval += int64(strategy.MinTimeInterval)
	return time.Duration(randTimeInterval) * time.Second
}
