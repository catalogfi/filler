package filler

import (
	"math/big"
)

type Strategy struct {
	makers    []string
	minAmount *big.Int
	maxAmount *big.Int
	orderPair string
	price     float64 //fee(bips) converted to price
}

func NewStrategy(makers []string, minAmount *big.Int, maxAmount *big.Int, orderPair string, price float64) *Strategy {
	return &Strategy{
		makers:    makers,
		minAmount: minAmount,
		maxAmount: maxAmount,
		orderPair: orderPair,
		price:     price,
	}
}

func StrategyWithDefaults(orderPair string) *Strategy {
	Fee := 10
	return &Strategy{
		makers:    make([]string, 0),
		minAmount: big.NewInt(0),
		maxAmount: big.NewInt(0),
		orderPair: orderPair,
		price:     float64(10000) / float64(10000-Fee), // fee's should be converted to price (fee is in bips)
	}
}
