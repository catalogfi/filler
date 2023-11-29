package handlers

import (
	"encoding/hex"
	"fmt"

	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"github.com/ethereum/go-ethereum/crypto"
)

func List(cfg types.CoreConfig, params types.RequestListOrders) ([]model.Order, error) {

	privKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	orders, err := rest.NewClient(fmt.Sprintf("https://%s", cfg.EnvConfig.OrderBook), hex.EncodeToString(crypto.FromECDSA(privKey))).GetOrders(rest.GetOrdersFilter{
		Maker:      params.Maker,
		OrderPair:  params.OrderPair,
		SecretHash: params.SecretHash,
		OrderBy:    params.OrderPair,
		MinPrice:   params.MinPrice,
		MaxPrice:   params.MaxPrice,
		Page:       int(params.Page),
		PerPage:    int(params.PerPage),
		Verbose:    true,
		Status:     int(model.Created),
	})
	if err != nil {
		return nil, err
	}

	return orders, nil
}
