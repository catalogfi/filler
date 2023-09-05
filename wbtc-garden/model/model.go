package model

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type Network map[Chain]NetworkConfig
type NetworkConfig struct {
	Assets      map[Asset]Token
	RPC         map[string]string
	IWRPC       string
	Expiry      int64
	EventWindow int64
}
type InstantWalletConfig struct {
	Dialector gorm.Dialector
}
type Config struct {
	Network    Network
	MinTxLimit string
	MaxTxLimit string
	DailyLimit string
	PriceTTL   int64
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

func ValidateIWCOnfig(iwConfig []InstantWalletConfig) bool {
	return iwConfig != nil && len(iwConfig) != 0 && iwConfig[0].Dialector != nil
}

func (c Chain) IsEVM() bool {
	return c == Ethereum || c == EthereumSepolia || c == EthereumLocalnet || c == EthereumOptimism
}

func (c Chain) IsBTC() bool {
	return c == Bitcoin || c == BitcoinTestnet || c == BitcoinRegtest
}

func (c Chain) IsTestnet() bool {
	return c == EthereumSepolia || c == EthereumLocalnet || c == BitcoinTestnet || c == BitcoinRegtest
}

type Token struct {
	Oracle       string
	TokenAddress string
	Decimals     int64
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
	Created
	Filled
	Executed
	FailedSoft
	FailedHard
	Cancelled
)

type SwapStatus uint

const (
	NotStarted SwapStatus = iota
	Detected
	Initiated
	Expired
	Redeemed
	Refunded
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

	Status               SwapStatus `json:"swapStatus"`
	SecretHash           string     `json:"secretHash"`
	Secret               string     `json:"secret"`
	OnChainIdentifier    string     `json:"onChainIdentifier"`
	InitiatorAddress     string     `json:"initiatorAddress"`
	RedeemerAddress      string     `json:"redeemerAddress"`
	Timelock             string     `json:"timelock"`
	Chain                Chain      `json:"chain"`
	Asset                Asset      `json:"asset"`
	Amount               string     `json:"amount"`
	FilledAmount         string     `json:"filledAmount"`
	InitiateTxHash       string     `json:"initiateTxHash" `
	RedeemTxHash         string     `json:"redeemTxHash" `
	RefundTxHash         string     `json:"refundTxHash" `
	PriceByOracle        float64    `json:"priceByOracle"`
	MinimumConfirmations uint64     `json:"minimumConfirmations"`
	CurrentConfirmations uint64     `json:"currentConfirmation"`
	InitiateBlockNumber  uint64     `json:"initiateBlockNumber"`
	IsInstantWallet      bool       `json:"-"`
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
	if _, ok := conf.Assets[asset]; ok {
		return nil
	}
	return fmt.Errorf("asset %v is not supported", asset)
}

func CompareOrder(a, b Order) bool {
	return a.Status == b.Status &&
		a.InitiatorAtomicSwap.CurrentConfirmations == b.InitiatorAtomicSwap.CurrentConfirmations &&
		a.FollowerAtomicSwap.CurrentConfirmations == b.FollowerAtomicSwap.CurrentConfirmations &&
		a.InitiatorAtomicSwap.FilledAmount == b.InitiatorAtomicSwap.FilledAmount &&
		a.FollowerAtomicSwap.FilledAmount == b.FollowerAtomicSwap.FilledAmount
}

type Blacklist struct {
	gorm.Model
	Address string `gorm:"unique"`
}

type LockedAmount struct {
	Asset  string
	Amount sql.NullInt64
}
