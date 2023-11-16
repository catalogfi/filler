package blockchain

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/cobi/pkg/swapper"
	"github.com/catalogfi/cobi/pkg/swapper/bitcoin"
	"github.com/catalogfi/cobi/pkg/swapper/ethereum"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
)

// The function `LoadClient` returns a client for a given blockchain chain and its corresponding URLs(set during config).
func LoadClient(chain model.Chain, config model.Network, iwConfig ...bitcoin.InstantWalletConfig) (interface{}, error) {
	if chain.IsBTC() {
		indexers := []bitcoin.Indexer{}
		for iType, url := range config[chain].RPC {
			switch iType {
			case "blockstream":
				indexers = append(indexers, bitcoin.NewBlockstream(url))
			case "mempool":
				indexers = append(indexers, bitcoin.NewMempool(url))
			default:
				return nil, fmt.Errorf("unknown indexer: %s", iType)
			}
		}
		indexer, err := bitcoin.NewMultiIndexer(indexers...)
		if err != nil {
			return nil, fmt.Errorf("failed to create indexer: %v", err)
		}
		client := bitcoin.NewClient(indexer, GetParams(chain))
		if len(iwConfig) > 0 {
			return bitcoin.InstantWalletWrapper(client, iwConfig[0]), nil
		}
		return client, nil
	}
	if chain.IsEVM() {
		logger, _ := zap.NewDevelopment()
		return ethereum.NewClient(logger, config[chain].RPC["ethrpc"])
	}
	return nil, fmt.Errorf("invalid chain: %s", chain)
}

// The function `LoadInitiatorSwap` loads an initiator swap based on the given atomic swap details, private key, secret hash, and URLs.
// initiateSwap can be used to construct a Swap Object with methods required to handle Atomicswap on initiator side.
func LoadInitiatorSwap(atomicSwap model.AtomicSwap, initiatorPrivateKey interface{}, secretHash string, config model.Network, minConfirmations uint64, iwConfig ...bitcoin.InstantWalletConfig) (swapper.InitiatorSwap, error) {
	client, err := LoadClient(atomicSwap.Chain, config, iwConfig...)
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
		return ethereum.NewInitiatorSwap(initiatorPrivateKey.(*ecdsa.PrivateKey), redeemerAddress.(common.Address), contractAddr, secHash, expiry, big.NewInt(int64(minConfirmations)), amt, client, config[atomicSwap.Chain].EventWindow)
	default:
		return nil, fmt.Errorf("unknown chain: %T", client)
	}
}

func LoadWatcher(atomicSwap model.AtomicSwap, secretHash string, config model.Network, minConfirmations uint64) (swapper.Watcher, error) {
	client, err := LoadClient(atomicSwap.Chain, config)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	initiatorAddress, err := ParseAddress(client, atomicSwap.InitiatorAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the initiator address: %s on chain: %v, %w", atomicSwap.InitiatorAddress, atomicSwap.Chain, err)
	}

	redeemerAddress, err := ParseAddress(client, atomicSwap.RedeemerAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the redeemer address: %s on chain: %v, %w", atomicSwap.RedeemerAddress, atomicSwap.Chain, err)
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
	case bitcoin.Client, bitcoin.InstantClient:
		htlcScript, err := bitcoin.NewHTLCScript(initiatorAddress.(btcutil.Address), redeemerAddress.(btcutil.Address), secHash, expiry.Int64())
		if err != nil {
			return nil, fmt.Errorf("failed to create HTLC script: %w", err)
		}

		witnessProgram := sha256.Sum256(htlcScript)
		scriptAddr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], client.(bitcoin.Client).Net())
		if err != nil {
			return nil, fmt.Errorf("failed to create script address: %w", err)
		}
		return bitcoin.NewWatcher(scriptAddr, expiry.Int64(), minConfirmations, amt.Uint64(), config[atomicSwap.Chain].IWRPC, client.(bitcoin.Client))
	case ethereum.Client:
		contractAddr := common.HexToAddress(atomicSwap.Asset.SecondaryID())
		orderId := sha256.Sum256(append(secHash, common.HexToAddress(atomicSwap.InitiatorAddress).Hash().Bytes()...))
		return ethereum.NewWatcher(contractAddr, secHash, orderId[:], expiry, big.NewInt(int64(minConfirmations)), amt, client, config[atomicSwap.Chain].EventWindow)
	default:
		return nil, fmt.Errorf("unknown chain: %T", client)
	}
}

func LoadRedeemerSwap(atomicSwap model.AtomicSwap, redeemerPrivateKey interface{}, secretHash string, config model.Network, minConfirmations uint64, iwConfig ...bitcoin.InstantWalletConfig) (swapper.RedeemerSwap, error) {
	client, err := LoadClient(atomicSwap.Chain, config, iwConfig...)
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
		return ethereum.NewRedeemerSwap(redeemerPrivateKey.(*ecdsa.PrivateKey), initiatorAddress.(common.Address), contractAddr, secHash, expiry, amt, big.NewInt(int64(minConfirmations)), client, config[atomicSwap.Chain].EventWindow)
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

func GetParams(chain model.Chain) *chaincfg.Params {
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

func CheckAddress(chain model.Chain, address string) error {
	if chain.IsEVM() {
		if address[:2] == "0x" {
			if len(address) != 42 {
				return fmt.Errorf("invalid evm (%v) address: %v", chain, address)
			}
		} else {
			if len(address) == 40 {
				return fmt.Errorf("invalid evm (%v) address: %v", chain, address)
			}
		}
	} else if chain.IsBTC() {
		_, err := btcutil.DecodeAddress(address, GetParams(chain))
		if err != nil {
			return fmt.Errorf("invalid bitcoin (%v) address: %v", chain, address)
		}
	} else {
		return fmt.Errorf("unknown chain: %v", chain)
	}
	return nil
}

func CheckHash(hash string) error {
	if len(hash) >= 2 && hash[0] == '0' && (hash[1] == 'x' || hash[1] == 'X') {
		hash = hash[2:]
	}
	_, err := hex.DecodeString(hash)
	if err != nil {
		return fmt.Errorf("not a valid hash %s", hash)
	}
	return nil
}

// value is in USD
func GetMinConfirmations(value *big.Int, chain model.Chain) uint64 {
	if chain.IsTestnet() {
		return 0
	}
	if chain.IsBTC() {
		switch {
		case value.Cmp(big.NewInt(10000)) < 1:
			return 1

		case value.Cmp(big.NewInt(100000)) < 1:
			return 2

		case value.Cmp(big.NewInt(1000000)) < 1:
			return 4

		case value.Cmp(big.NewInt(10000000)) < 1:
			return 6

		case value.Cmp(big.NewInt(100000000)) < 1:
			return 8

		default:
			return 12
		}
	} else if chain.IsEVM() {
		switch {
		case value.Cmp(big.NewInt(10000)) < 1:
			return 6

		case value.Cmp(big.NewInt(100000)) < 1:
			return 12

		case value.Cmp(big.NewInt(1000000)) < 1:
			return 18

		case value.Cmp(big.NewInt(10000000)) < 1:
			return 24

		case value.Cmp(big.NewInt(100000000)) < 1:
			return 30

		default:
			return 100
		}
	}
	return 0
}

type Price struct {
	Price     float64
	Timestamp int64
}

func GetPrice(oracle string) (Price, error) {
	resp, err := http.Get(oracle)
	if err != nil {
		return Price{}, fmt.Errorf("failed to build get request: %v", err)
	}
	defer resp.Body.Close()
	var apiResponse struct {
		Data      map[string]interface{} `json:"data"`
		Timestamp int64                  `json:"timestamp"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return Price{}, fmt.Errorf("failed to decode response: %v", err)
	}
	priceUsdStr, ok := apiResponse.Data["priceUsd"].(string)
	if !ok {
		return Price{}, fmt.Errorf("failed to parse price from: %v", apiResponse.Data)
	}
	priceUsd, err := strconv.ParseFloat(priceUsdStr, 64)
	if err != nil {
		return Price{}, fmt.Errorf("failed to convert priceUsd to float64: %v", err)
	}
	return Price{priceUsd, apiResponse.Timestamp}, nil
}
