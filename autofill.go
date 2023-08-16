package cobi

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
)

type FillStrategy struct {
	Makers        []string        `json:"makers"`
	MinAmount     float64         `json:"minAmount"`
	MaxAmount     float64         `json:"maxAmount"`
	OrderPair     string          `json:"orderPair"`
	PriceStrategy json.RawMessage `json:"strategy"`

	Strategy PriceStrategy
}

func AutoFill(entropy []byte, store Store, config model.Config) *cobra.Command {
	var (
		url      string
		account  uint32
		strategy string
	)
	var cmd = &cobra.Command{
		Use:   "autofill",
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
			taker := crypto.PubkeyToAddress(privKey.PublicKey)

			data, err := os.ReadFile(strategy)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("error while reading strategy.json: %v", err))
			}

			var strategy FillStrategy
			err = json.Unmarshal(data, &strategy)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("error while unmarshalling strategy.json: %v", err))
			}
			strategy.Strategy, err = UnmarshalPriceStrategy(strategy.PriceStrategy)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("error while unmarshalling strategy.json: %v", err))
			}

			client := rest.NewClient(url, privKey.D.Text(16))
			token, err := client.Login()
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while getting the signing key: %v", err))
			}
			if err := client.SetJwt(token); err != nil {
				cobra.CheckErr(fmt.Sprintf("Error to parse signing key: %v", err))
			}

			for {
				price, err := strategy.Strategy.Price()
				if err != nil {
					fmt.Println(fmt.Sprintf("Error while parsing order pair: %v", err))
					continue
				}

				orders, err := client.GetOrders(rest.GetOrdersFilter{
					Maker:     strings.Join(strategy.Makers, ","),
					OrderPair: strategy.OrderPair,
					MinPrice:  price,
					MaxPrice:  math.MaxFloat64,
					Status:    int(model.OrderCreated),
				})
				if err != nil {
					fmt.Println(fmt.Sprintf("Error while parsing order pair: %v", err))
					continue
				}

				for _, order := range orders {
					fromChain, toChain, _, toAsset, err := model.ParseOrderPair(order.OrderPair)
					if err != nil {
						fmt.Println(fmt.Sprintf("Error while parsing order pair: %v", err))
						continue
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

					balance, err := getVirtualBalance(toChain, toAsset, strategy.OrderPair, config, client, taker.Hex(), fromAddress, true)
					if err != nil {
						fmt.Println(fmt.Sprintf("Error failed to get virtual balance: %v", err))
						continue
					}

					orderAmount, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
					if !ok {
						fmt.Println(fmt.Sprintf("Error failed to get order amount: %v", err))
						continue
					}

					if balance.Cmp(orderAmount) < 0 {
						fmt.Println(fmt.Sprintf("Error insufficient balance have %v: need %v", balance.String(), orderAmount.String()))
						continue
					}

					if err := client.FillOrder(order.ID, fromAddress, toAddress); err != nil {
						fmt.Println(fmt.Sprintf("Error while Filling the Order: %v with OrderID %d cross ❌", err, order.ID))
						continue
					}
					if err = store.PutSecretHash(order.SecretHash, uint64(order.ID)); err != nil {
						fmt.Println(fmt.Sprintf("Error while storing secret hash: %v", err))
						continue
					}
					fmt.Printf("Filled order %d ✅", order.ID)
				}
				time.Sleep(10 * time.Second)
			}
		}}

	cmd.Flags().StringVar(&url, "url", "", "config file (default is ./config.json)")
	cmd.MarkFlagRequired("url")
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().StringVar(&strategy, "strategy", "../../strategy.json", "config file (default: ./strategy.json)")
	return cmd
}

func getVirtualBalance(chain model.Chain, asset model.Asset, op string, config model.Config, client rest.Client, signer, address string, isFill bool) (*big.Int, error) {
	balance, err := getBalance(chain, address, config, asset)
	if err != nil {
		return nil, err
	}

	fillOrders, err := client.GetOrders(rest.GetOrdersFilter{
		Taker:     signer,
		OrderPair: op,
		Status:    int(model.OrderFilled),
		Verbose:   true,
	})
	if err != nil {
		return nil, err
	}
	createOrders, err := client.GetOrders(rest.GetOrdersFilter{
		Maker:     signer,
		OrderPair: op,
		Status:    int(model.OrderCreated),
		Verbose:   true,
	})
	if err != nil {
		return nil, err
	}
	orders := append(fillOrders, createOrders...)

	commitedAmount := big.NewInt(0)
	for _, order := range orders {
		orderAmt, ok := new(big.Int).SetString(order.FollowerAtomicSwap.Amount, 10)
		if !ok {
			return nil, err
		}
		commitedAmount.Add(commitedAmount, orderAmt)
	}

	return new(big.Int).Sub(balance, commitedAmount), nil
}
