package blockchain

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/swapper"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
)

// The function `LoadClient` returns a client for a given blockchain chain and its corresponding URLs(set during config).
func LoadClient(chain model.Chain, config model.Config) (interface{}, error) {
	if chain.IsBTC() {
		return bitcoin.NewClient(config[chain].RPC, getParams(chain)), nil
	}
	if chain.IsEVM() {
		logger, _ := zap.NewDevelopment()
		return ethereum.NewClient(logger, config[chain].RPC)
	}
	return nil, fmt.Errorf("invalid chain: %s", chain)
}

// The function `LoadInitiatorSwap` loads an initiator swap based on the given atomic swap details, private key, secret hash, and URLs.
// initiateSwap can be used to construct a Swap Object with methods required to handle Atomicswap on initiator side.
func LoadInitiatorSwap(atomicSwap model.AtomicSwap, initiatorPrivateKey interface{}, secretHash string, config model.Config, minConfirmations uint64) (swapper.InitiatorSwap, error) {
	client, err := LoadClient(atomicSwap.Chain, config)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	redeemerAddress, err := ParseAddress(client, atomicSwap.RedeemerAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	secHash, err := hex.DecodeString(secretHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	amt, ok := new(big.Int).SetString(atomicSwap.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount: %s", atomicSwap.Amount)
	}

	expiry, ok := new(big.Int).SetString(atomicSwap.Timelock, 10)
	if !ok {
		return nil, fmt.Errorf("invalid timelock: %s", atomicSwap.Timelock)
	}
	logger, _ := zap.NewDevelopment()

	switch client := client.(type) {
	case bitcoin.Client:
		return bitcoin.NewInitiatorSwap(logger, initiatorPrivateKey.(*btcec.PrivateKey), redeemerAddress.(btcutil.Address), secHash, expiry.Int64(), minConfirmations, amt.Uint64(), client)
	case ethereum.Client:
		contractAddr := common.HexToAddress(atomicSwap.Asset.SecondaryID())
		return ethereum.NewInitiatorSwap(initiatorPrivateKey.(*ecdsa.PrivateKey), redeemerAddress.(common.Address), contractAddr, secHash, expiry, big.NewInt(int64(minConfirmations)), amt, client)
	default:
		return nil, fmt.Errorf("unknown chain: %T", client)
	}
}

func LoadWatcher(atomicSwap model.AtomicSwap, secretHash string, config model.Config, minConfirmations uint64) (swapper.Watcher, error) {
	client, err := LoadClient(atomicSwap.Chain, config)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	initiatorAddress, err := ParseAddress(client, atomicSwap.InitiatorAddress)
	if err != nil {
		return nil, err
	}

	redeemerAddress, err := ParseAddress(client, atomicSwap.RedeemerAddress)
	if err != nil {
		return nil, err
	}

	secHash, err := hex.DecodeString(secretHash)
	if err != nil {
		return nil, err
	}

	amt, ok := new(big.Int).SetString(atomicSwap.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount: %s", atomicSwap.Amount)
	}

	expiry, ok := new(big.Int).SetString(atomicSwap.Timelock, 10)
	if !ok {
		return nil, fmt.Errorf("invalid timelock: %s", atomicSwap.Timelock)
	}

	switch client := client.(type) {
	case bitcoin.Client:
		htlcScript, err := bitcoin.NewHTLCScript(initiatorAddress.(btcutil.Address), redeemerAddress.(btcutil.Address), secHash, expiry.Int64())
		if err != nil {
			return nil, fmt.Errorf("failed to create HTLC script: %w", err)
		}

		witnessProgram := sha256.Sum256(htlcScript)
		scriptAddr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], client.Net())
		if err != nil {
			return nil, fmt.Errorf("failed to create script address: %w", err)
		}
		return bitcoin.NewWatcher(scriptAddr, expiry.Int64(), minConfirmations, amt.Uint64(), client)
	case ethereum.Client:
		contractAddr := common.HexToAddress(atomicSwap.Asset.SecondaryID())
		orderId := sha256.Sum256(append(secHash, common.HexToAddress(atomicSwap.InitiatorAddress).Hash().Bytes()...))
		return ethereum.NewWatcher(contractAddr, secHash, orderId[:], expiry, big.NewInt(int64(minConfirmations)), amt, client)
	default:
		return nil, fmt.Errorf("unknown chain: %T", client)
	}
}

func LoadRedeemerSwap(atomicSwap model.AtomicSwap, redeemerPrivateKey interface{}, secretHash string, config model.Config, minConfirmations uint64) (swapper.RedeemerSwap, error) {
	client, err := LoadClient(atomicSwap.Chain, config)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	initiatorAddress, err := ParseAddress(client, atomicSwap.InitiatorAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	secHash, err := hex.DecodeString(secretHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	amt, ok := new(big.Int).SetString(atomicSwap.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount: %s", atomicSwap.Amount)
	}

	expiry, ok := new(big.Int).SetString(atomicSwap.Timelock, 10)
	if !ok {
		return nil, fmt.Errorf("invalid timelock: %s", atomicSwap.Timelock)
	}

	logger, _ := zap.NewDevelopment()
	switch client := client.(type) {
	case bitcoin.Client:
		return bitcoin.NewRedeemerSwap(logger, redeemerPrivateKey.(*btcec.PrivateKey), initiatorAddress.(btcutil.Address), secHash, expiry.Int64(), minConfirmations, amt.Uint64(), client)
	case ethereum.Client:
		contractAddr := common.HexToAddress(atomicSwap.Asset.SecondaryID())
		return ethereum.NewRedeemerSwap(redeemerPrivateKey.(*ecdsa.PrivateKey), initiatorAddress.(common.Address), contractAddr, secHash, expiry, amt, big.NewInt(int64(minConfirmations)), client)
	default:
		return nil, fmt.Errorf("unknown chain: %T", client)
	}
}

func ParseKey(chain model.Chain, key string) (interface{}, error) {
	switch chain {
	case model.Bitcoin:
		privKeyBytes, err := hex.DecodeString(key)
		if err != nil {
			return nil, err
		}
		// ignoring public key as we do not need it
		privKey, _ := btcec.PrivKeyFromBytes(privKeyBytes)
		return privKey, nil
	case model.Ethereum:
		return crypto.HexToECDSA(key)
	default:
		return nil, fmt.Errorf("unknown chain: %s", chain)
	}
}

func ParseAddress(client interface{}, address string) (interface{}, error) {
	switch client := client.(type) {
	case bitcoin.Client:
		return btcutil.DecodeAddress(address, client.Net())
	case ethereum.Client:
		return common.HexToAddress(address), nil
	default:
		return nil, fmt.Errorf("unknown chain: %T", client)
	}
}

func getParams(chain model.Chain) *chaincfg.Params {
	switch chain {
	case model.Bitcoin:
		return &chaincfg.MainNetParams
	case model.BitcoinTestnet:
		return &chaincfg.TestNet3Params
	case model.BitcoinRegtest:
		return &chaincfg.RegressionNetParams
	default:
		panic("constraint violation: unknown chain")
	}
}
