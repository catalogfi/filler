package filler

import (
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/common"
)

// Strategies is a list of strategy for different order pairs.
type Strategies []Strategy

// Strategy defines the criteria of whether an order should be filled by the Filler. It is basing on the order pair, and
// each order pair will have its own strategy.
type Strategy struct {
	OrderPair string
	Makers    []string // whitelisted makers, nil means allowing any address
	MinAmount *big.Int // minimum amount, nil means no minimum requirement
	MaxAmount *big.Int // maximum amount, nil means no maximum requirement
	Fee       int      // fee in basic point (0.01%)
}

// NewStrategy returns a new strategy with
func NewStrategy(orderPair, send, receive string, makers []string, minAmount *big.Int, maxAmount *big.Int, fee int) (Strategy, error) {
	// Validate the order pair and addresses.
	receiveChain, sendChain, _, _, err := model.ParseOrderPair(orderPair)
	if err != nil {
		return Strategy{}, err
	}
	// Since the fromChain is relative to the order maker, so we need to check our from address.
	if err := ValidateAddress(sendChain, send); err != nil {
		return Strategy{}, err
	}
	if err := ValidateAddress(receiveChain, receive); err != nil {
		return Strategy{}, err
	}

	return Strategy{
		OrderPair: orderPair,
		Makers:    makers,
		MinAmount: minAmount,
		MaxAmount: maxAmount,
		Fee:       fee,
	}, nil
}

// Price when considering fees. price = 1 / (1-fee)
func (strategy Strategy) Price() float64 {
	return float64(10000) / float64(10000-strategy.Fee)
}

// Match checks if the given order matches our strategy. It also gives an error to indicate the unmatched reason.
func (strategy Strategy) Match(order model.Order) (bool, error) {
	// Check price
	if order.Price < strategy.Price() {
		return false, fmt.Errorf("price too low, %v < %v", order.Price, strategy.Price())
	}

	// Check if the maker is whitelisted
	if len(strategy.Makers) != 0 {
		hasMaker := false
		for _, maker := range strategy.Makers {
			if maker == order.Maker {
				hasMaker = true
				break
			}
		}
		if !hasMaker {
			return false, fmt.Errorf("maker [%v] not whitelised", order.Maker)
		}
	}

	// Check if order amount is in the expect range
	orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
	if !ok {
		return false, fmt.Errorf("invalid order amount = %v", order.FollowerAtomicSwap.Amount)
	}
	if strategy.MinAmount != nil && orderAmount.Cmp(strategy.MinAmount) < 0 {
		return false, fmt.Errorf("amount(%v) lower than minimum(%v)", orderAmount.String(), strategy.MinAmount.String())
	}
	if strategy.MaxAmount != nil && orderAmount.Cmp(strategy.MaxAmount) > 0 {
		return false, fmt.Errorf("amount(%v) greater than maximum(%v)", orderAmount.String(), strategy.MaxAmount.String())
	}
	return true, nil
}

func ValidateAddress(chain model.Chain, address string) error {
	if chain.IsEVM() {
		if !common.IsHexAddress(address) {
			return fmt.Errorf("invalid evm (%v) address: %v", chain, address)
		}
		return nil
	} else if chain.IsBTC() {
		_, err := btcutil.DecodeAddress(address, chain.Params())
		return err
	} else {
		return fmt.Errorf("unknown chain: %v", chain)
	}
}
