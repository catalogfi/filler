package util

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
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

func BtcecToECDSA(key *btcec.PrivateKey) (*ecdsa.PrivateKey, error) {
	return crypto.ToECDSA(key.Serialize())
}

func EcdsaToBtcec(key *ecdsa.PrivateKey) *btcec.PrivateKey {
	pk, _ := btcec.PrivKeyFromBytes(crypto.FromECDSA(key))
	return pk
}

type WsClientDialer = func() rest.WSClient
