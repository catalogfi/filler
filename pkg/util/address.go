package util

import (
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/common"
)

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
