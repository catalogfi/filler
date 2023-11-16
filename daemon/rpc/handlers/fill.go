package handlers

import (
	"encoding/hex"
	"fmt"

	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/crypto"
)

func FillOrder(cfg types.CoreConfig, params types.RequestFill) error {
	key, err := cfg.Keys.GetKey(model.Ethereum, params.UserAccount, 0)
	if err != nil {
		return fmt.Errorf("error while getting the signing key: %v", err)
	}
	privKey, err := key.ECDSA()
	if err != nil {
		return fmt.Errorf("error while getting the private key: %v", err)
	}
	client := rest.NewClient(fmt.Sprintf("https://%s", cfg.EnvConfig.OrderBook), hex.EncodeToString(crypto.FromECDSA(privKey)))
	token, err := client.Login()
	if err != nil {
		return fmt.Errorf("error while logging in : %v", err)
	}
	if err := client.SetJwt(token); err != nil {
		return fmt.Errorf("error while setting the JWT: %v", err)
	}
	userStore := cfg.Storage.UserStore(params.UserAccount)

	order, err := client.GetOrder(uint(params.OrderId))
	if err != nil {
		return fmt.Errorf("error while getting the  order pair: %v", err)
	}

	toChain, fromChain, _, _, err := model.ParseOrderPair(order.OrderPair)
	if err != nil {
		return fmt.Errorf("error while parsing order pair: %v", err)
	}

	// Get the addresses on different chains.
	fromKey, err := cfg.Keys.GetKey(fromChain, params.UserAccount, 0)
	if err != nil {
		return fmt.Errorf("error while getting from key: %v", err)
	}
	fromAddress, err := fromKey.Address(fromChain, cfg.EnvConfig.Network, false)
	if err != nil {
		return fmt.Errorf("error while getting address string: %v", err)
	}
	toKey, err := cfg.Keys.GetKey(toChain, params.UserAccount, 0)
	if err != nil {
		return fmt.Errorf("error while getting to key: %v", err)
	}
	toAddress, err := toKey.Address(toChain, cfg.EnvConfig.Network, true)
	if err != nil {
		return fmt.Errorf("error while getting address string: %v", err)
	}

	if err := client.FillOrder(uint(params.OrderId), fromAddress, toAddress); err != nil {
		return fmt.Errorf("error while filling the order: %v", err)
	}
	if err = userStore.PutSecretHash(order.SecretHash, uint64(params.OrderId)); err != nil {
		return fmt.Errorf("error while storing the secret hash: %v", err)
	}

	// fmt.Println("Order filled successfully")
	return nil
}
