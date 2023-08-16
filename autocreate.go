package cobi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
)

type CreateStrategy struct {
	MinTimeInterval uint32          `json:"minTimeInterval"`
	MaxTimeInterval uint32          `json:"maxTimeInterval"`
	MinAmount       string          `json:"minAmount"`
	MaxAmount       string          `json:"maxAmount"`
	OrderPair       string          `json:"orderPair"`
	PriceStrategy   json.RawMessage `json:"strategy"`

	MinAmt   *big.Int
	MaxAmt   *big.Int
	Strategy PriceStrategy
}

func AutoCreate(entropy []byte, store Store, config model.Config) *cobra.Command {
	var (
		url      string
		account  uint32
		strategy string
	)
	var cmd = &cobra.Command{
		Use:   "autocreate",
		Short: "fills the Orders based on strategy provided",
		Run: func(c *cobra.Command, args []string) {
			// Load keys
			keys := NewKeys()
			key, err := keys.GetKey(entropy, model.Ethereum, account, 0)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting the signing key: %v", err))
			}
			privKey, err := key.ECDSA()
			if err != nil {
				cobra.CheckErr(err)
			}
			maker := crypto.PubkeyToAddress(privKey.PublicKey)
			client := rest.NewClient(url, privKey.D.Text(16))

			data, err := os.ReadFile(strategy)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("error while reading strategy.json: %v", err))
			}

			var strategy CreateStrategy
			err = json.Unmarshal(data, &strategy)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("error while unmarshalling strategy.json: %v", err))
			}

			strategy.Strategy, err = UnmarshalPriceStrategy(strategy.PriceStrategy)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("error while unmarshalling strategy.json: %v", err))
			}

			var ok bool
			strategy.MinAmt, ok = new(big.Int).SetString(strategy.MinAmount, 10)
			if !ok {
				cobra.CheckErr(fmt.Errorf("error while unmarshalling strategy.json (invalud minimum amount): %v", err))
			}
			strategy.MaxAmt, ok = new(big.Int).SetString(strategy.MaxAmount, 10)
			if !ok {
				cobra.CheckErr(fmt.Errorf("error while unmarshalling strategy.json (invalud minimum amount): %v", err))
			}

			token, err := client.Login()
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting the signing key: %v", err))

			}
			if err := client.SetJwt(token); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error to parse signing key: %v", err))
			}
			for {
				randTimeInterval, err := rand.Int(rand.Reader, big.NewInt(int64(strategy.MaxTimeInterval-strategy.MinTimeInterval)))
				if err != nil {
					cobra.CheckErr(fmt.Errorf("error can't create a random value: %v", err))
				}
				randTimeInterval.Add(randTimeInterval, big.NewInt(int64(strategy.MinTimeInterval)))

				randAmount, err := rand.Int(rand.Reader, new(big.Int).Sub(strategy.MaxAmt, strategy.MinAmt))
				if err != nil {
					cobra.CheckErr(fmt.Errorf("error can't create a random value: %v", err))
				}
				randAmount.Add(randAmount, strategy.MinAmt)

				fromChain, toChain, fromAsset, _, err := model.ParseOrderPair(strategy.OrderPair)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while parsing order pair: %v", err))
					return
				}

				// Get the addresses on different chains.
				fromKey, err := keys.GetKey(entropy, fromChain, account, 0)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while getting from key: %v", err))
					return
				}
				fromAddress, err := fromKey.Address(fromChain)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
					return
				}
				toKey, err := keys.GetKey(entropy, fromChain, account, 0)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while getting to key: %v", err))
					return
				}
				toAddress, err := toKey.Address(toChain)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
					return
				}

				balance, err := getVirtualBalance(fromChain, fromAsset, strategy.OrderPair, config, client, maker.Hex(), fromAddress, false)
				if err != nil {
					fmt.Println(fmt.Sprintf("Error failed to get virtual balance: %v", err))
					continue
				}

				if balance.Cmp(randAmount) < 0 {
					fmt.Println(fmt.Sprintf("Error insufficient balance have %v: need %v", balance.String(), randAmount.String()))
					continue
				}

				receiveAmount, err := strategy.Strategy.CalculatereceiveAmount(randAmount)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while getting address string: %v", err))
					return
				}

				secret := [32]byte{}
				_, err = rand.Read(secret[:])
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while creating order: %v", err))
					return
				}
				secretHash := sha256.Sum256(secret[:])

				id, err := client.CreateOrder(fromAddress, toAddress, strategy.OrderPair, randAmount.String(), receiveAmount.String(), hex.EncodeToString(secretHash[:]))
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while creating order: %v", err))
					return
				}

				if err := store.PutSecret(hex.EncodeToString(secretHash[:]), hex.EncodeToString(secret[:]), uint64(id)); err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while creating order: %v", err))
					return
				}

				time.Sleep(time.Duration(randTimeInterval.Int64()) * time.Second)
			}
		},
	}

	cmd.Flags().StringVar(&url, "url", "", "config file (default is ./config.json)")
	cmd.MarkFlagRequired("url")
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().StringVar(&strategy, "strategy", "../../strategy.json", "config file (default: ./strategy.json)")
	return cmd
}

type PriceStrategy interface {
	Price() (float64, error)
	CalculatereceiveAmount(val *big.Int) (*big.Int, error)
}

type JSONObj struct {
	StrategyType string          `json:"strategyType"`
	Strategy     json.RawMessage `json:"strategy"`
}

func MarshalPriceStrategy(strategy PriceStrategy) ([]byte, error) {
	var obj JSONObj
	var err error

	switch strategy := strategy.(type) {
	case Likewise:
		obj.StrategyType = "likewise"
		obj.Strategy, err = json.Marshal(strategy)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(obj)
}

func UnmarshalPriceStrategy(data []byte) (PriceStrategy, error) {
	var obj JSONObj
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	switch obj.StrategyType {
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
	return big.NewInt(val.Int64() * int64(lw.Fee) / 10000), nil
}

func (lw Likewise) Price() (float64, error) {
	return float64(10000) / float64(10000-lw.Fee), nil
}

func NewLikewise(feeInBips uint64) (PriceStrategy, error) {
	return &Likewise{}, nil
}
