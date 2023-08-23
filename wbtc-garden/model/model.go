package model

import (
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"gorm.io/gorm"
)

type Config map[Chain]NetworkConfig
type NetworkConfig struct {
	Assets      map[Asset]bool
	RPC         string
	Expiry      int64
	EventWindow int64
}

type Chain string

const (
	Bitcoin          Chain = "bitcoin"
	BitcoinTestnet   Chain = "bitcoin_testnet"
	BitcoinRegtest   Chain = "bitcoin_regtest"
	Ethereum         Chain = "ethereum"
	EthereumSepolia  Chain = "ethereum_sepolia"
	EthereumLocalnet Chain = "ethereum_localnet"
	EthereumOptimism Chain = "ethereum_optimism"
)

func ParseChain(c string) (Chain, error) {
	switch strings.ToLower(c) {
	case "bitcoin":
		return Bitcoin, nil
	case "bitcoin_testnet", "bitcoin-testnet", "bitcoin-testnet3":
		return BitcoinTestnet, nil
	case "bitcoin_regtest", "bitcoin-regtest", "bitcoin-localnet":
		return BitcoinRegtest, nil
	case "ethereum":
		return Ethereum, nil
	case "ethereum_sepolia", "sepolia", "ethereum-sepolia":
		return EthereumSepolia, nil
	case "ethereum_localnet", "ethereum-localnet":
		return EthereumLocalnet, nil
	case "ethereum_optimism", "optimism", "ethereum-optimism":
		return EthereumOptimism, nil
	default:
		return Chain(""), fmt.Errorf("unknown chain %v", c)
	}
}

func (c Chain) IsEVM() bool {
	return c == Ethereum || c == EthereumSepolia || c == EthereumLocalnet || c == EthereumOptimism
}

func (c Chain) IsBTC() bool {
	return c == Bitcoin || c == BitcoinTestnet || c == BitcoinRegtest
}

type Asset string

const (
	Primary Asset = "primary"
)

func NewSecondary(address string) Asset {
	return Asset(address)
}

func (a Asset) SecondaryID() string {
	if string(a) == "primary" {
		return ""
	}
	return string(a)
}

type Status uint

const (
	Unknown Status = iota
	OrderCreated
	OrderFilled
	InitiatorAtomicSwapInitiated
	FollowerAtomicSwapInitiated
	FollowerAtomicSwapRedeemed
	InitiatorAtomicSwapRedeemed
	InitiatorAtomicSwapRefunded
	FollowerAtomicSwapRefunded
	OrderExecuted
	OrderFailedSoft
	OrderFailedHard
	OrderCancelled
)

type VerifySiwe struct {
	Message   string `json:"message" binding:"required"`
	Signature string `json:"signature" binding:"required"`
}

type Order struct {
	gorm.Model

	Maker     string `json:"maker"`
	Taker     string `json:"taker"`
	OrderPair string `json:"orderPair"`

	InitiatorAtomicSwapID uint
	FollowerAtomicSwapID  uint
	InitiatorAtomicSwap   *AtomicSwap `json:"initiatorAtomicSwap" gorm:"foreignKey:InitiatorAtomicSwapID"`
	FollowerAtomicSwap    *AtomicSwap `json:"followerAtomicSwap" gorm:"foreignKey:FollowerAtomicSwapID"`

	SecretHash           string  `json:"secretHash" gorm:"unique;not null"`
	Secret               string  `json:"secret"`
	Price                float64 `json:"price"`
	Status               Status  `json:"status"`
	SecretNonce          uint64  `json:"secretNonce"`
	UserBtcWalletAddress string  `json:"userBtcWalletAddress"`

	Fee uint `json:"fee"`
}

type AtomicSwap struct {
	gorm.Model

	InitiatorAddress          string  `json:"initiatorAddress"`
	RedeemerAddress           string  `json:"redeemerAddress"`
	Timelock                  string  `json:"timelock"`
	Chain                     Chain   `json:"chain"`
	Asset                     Asset   `json:"asset"`
	Amount                    string  `json:"amount"`
	InitiateTxHash            string  `json:"initiateTxHash" `
	RedeemTxHash              string  `json:"redeemTxHash" `
	RefundTxHash              string  `json:"refundTxHash" `
	PriceByOracle             float64 `json:"priceByOracle"`
	MinimumConfirmations      uint64  `json:"minimumConfirmations"`
	CurrentConfirmationStatus uint64  `json:"currentConfirmationStatus"`
	IsInstantWallet           bool    `json:"-"`
}

type LockedAmount struct {
	Asset  string
	Amount sql.NullInt64
}

func CombineAndAddAmount(arr1, arr2 []LockedAmount) []LockedAmount {
	combinedMap := make(map[string]sql.NullInt64)

	if len(arr1) == 0 {
		return arr2
	} else if len(arr2) == 0 {
		return arr1
	} else if len(arr1) == 0 && len(arr2) == 0 {
		return nil
	}
	for _, item := range arr1 {
		if _, ok := combinedMap[item.Asset]; ok {
			combinedMap[item.Asset] = sql.NullInt64{
				Int64: combinedMap[item.Asset].Int64 + item.Amount.Int64,
				Valid: true,
			}
		} else {
			combinedMap[item.Asset] = item.Amount
		}
	}

	for _, item := range arr2 {
		if _, ok := combinedMap[item.Asset]; ok {
			combinedMap[item.Asset] = sql.NullInt64{
				Int64: combinedMap[item.Asset].Int64 + item.Amount.Int64,
				Valid: true,
			}
		} else {
			combinedMap[item.Asset] = item.Amount
		}
	}

	var combinedArray []LockedAmount
	for asset, amount := range combinedMap {
		combinedArray = append(combinedArray, LockedAmount{
			Asset:  asset,
			Amount: amount,
		})
	}

	return combinedArray
}

type StringArray []string

func (sa StringArray) Value() (driver.Value, error) {
	return strings.Join(sa, ","), nil
}

func (sa *StringArray) Scan(value interface{}) error {
	if value == nil {
		*sa = make([]string, 0)
		return nil
	}

	switch v := value.(type) {
	case string:
		*sa = strings.Split(v, ",")
	case []byte:
		*sa = strings.Split(string(v), ",")
	default:
		return fmt.Errorf("unsupported data type for StringArray: %T", value)
	}

	return nil
}

func ParseOrderPair(orderPair string) (Chain, Chain, Asset, Asset, error) {
	chainAssets := strings.Split(orderPair, "-")
	if len(chainAssets) != 2 {
		return "", "", "", "", fmt.Errorf("failed to parse the order pair, should be of the format <chain>:<asset>-<chain>:<asset>. got: %v", orderPair)
	}
	sendChain, sendAsset, err := ParseChainAsset(chainAssets[0])
	if err != nil {
		return "", "", "", "", err
	}
	receiveChain, receiveAsset, err := ParseChainAsset(chainAssets[1])
	if err != nil {
		return "", "", "", "", err
	}
	return sendChain, receiveChain, sendAsset, receiveAsset, nil
}

func ParseChainAsset(chainAsset string) (Chain, Asset, error) {
	chainAndAsset := strings.Split(chainAsset, ":")
	if len(chainAndAsset) > 2 {
		return "", "", fmt.Errorf("failed to parse the chain and asset, should be of the format <chain>:<asset>. got: %v", chainAsset)
	}
	if len(chainAndAsset) == 1 {
		return Chain(chainAndAsset[0]), Primary, nil
	}
	return Chain(chainAndAsset[0]), NewSecondary(chainAndAsset[1]), nil
}

func (conf NetworkConfig) IsSupported(asset Asset) error {
	if conf.Assets[asset] {
		return nil
	}
	return fmt.Errorf("asset %v is not supported", asset)
}

func CompareOrderSlices(a, b []Order) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v.Status != b[i].Status {
			return false
		}
		if v.FollowerAtomicSwap.CurrentConfirmationStatus != b[i].FollowerAtomicSwap.CurrentConfirmationStatus || v.InitiatorAtomicSwap.CurrentConfirmationStatus != b[i].InitiatorAtomicSwap.CurrentConfirmationStatus {
			return false
		}
	}
	return true

}

func ValidateSecretHash(input string) error {
	decoded, err := hex.DecodeString(input)
	if err != nil {
		return errors.New("wrong secret hash: not a valid hexadecimal string")
	}
	if len(decoded) != 32 {
		return errors.New("wrong secret hash: length should be 32 bytes (64 characters)")
	}

	return nil
}

func ValidateEthereumAddress(input string) error {
	if len(input) > 2 && input[:2] == "0x" {
		input = input[2:]
	}
	if len(input) != 40 {
		return errors.New("wrong ethereum address: length should be 40 bytes")
	}
	_, err := hex.DecodeString(input)
	// fmt.Println("IsEthereumAddress", len(input))
	if err != nil {
		return errors.New("wrong ethereum address: not a valid hexadecimal string")
	}
	return nil
}

func ValidateBitcoinAddress(address string, chain Chain) error {
	chaincfg, err := GetParams(chain)
	if err != nil {
		return err
	}
	_, err = btcutil.DecodeAddress(address, chaincfg)
	return err
}

func GetParams(chain Chain) (*chaincfg.Params, error) {
	switch chain {
	case Bitcoin:
		return &chaincfg.MainNetParams, nil
	case BitcoinTestnet:
		return &chaincfg.TestNet3Params, nil
	case BitcoinRegtest:
		return &chaincfg.RegressionNetParams, nil
	default:
		return nil, errors.New("constraint violation: unknown chain")
	}
}
func CompareOrder(a, b Order) bool {
	return a.Status == b.Status &&
		a.InitiatorAtomicSwap.CurrentConfirmationStatus == b.InitiatorAtomicSwap.CurrentConfirmationStatus &&
		a.FollowerAtomicSwap.CurrentConfirmationStatus == b.FollowerAtomicSwap.CurrentConfirmationStatus
}
