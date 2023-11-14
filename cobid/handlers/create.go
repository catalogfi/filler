package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/catalogfi/cobi/cobid/types"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/crypto"
)

func Create(cfg types.CoreConfig, params types.RequestCreate) (uint, error) {

	err := types.CheckStrings(params.OrderPair, params.SendAmount, params.ReceiveAmount)
	if err != nil {
		return 0, fmt.Errorf("Error while parsing order pair: %v", err)
	}

	secret := [32]byte{}
	if _, err := rand.Read(secret[:]); err != nil {
		return 0, fmt.Errorf("Error while generating secret: %v", err)
	}

	hash := sha256.Sum256(secret[:])
	secretHash := hex.EncodeToString(hash[:])
	userStore := cfg.Storage.UserStore(params.UserAccount)
	key, err := cfg.Keys.GetKey(model.Ethereum, params.UserAccount, 0)
	if err != nil {
		return 0, fmt.Errorf("Error while getting the signing key: %v", err)
	}
	privKey, err := key.ECDSA()
	if err != nil {
		return 0, err
	}
	client := rest.NewClient(fmt.Sprintf("https://%s", cfg.EnvConfig.OrderBook), hex.EncodeToString(crypto.FromECDSA(privKey)))
	token, err := client.Login()
	if err != nil {
		return 0, fmt.Errorf("Error while getting the signing key: %v", err)
	}
	if err := client.SetJwt(token); err != nil {
		return 0, fmt.Errorf("Error to parse signing key: %v", err)
	}

	fromChain, toChain, _, _, err := model.ParseOrderPair(params.OrderPair)
	if err != nil {
		return 0, fmt.Errorf("Error while parsing order pair: %v", err)
	}

	// Get the addresses on different chains.
	fromKey, err := cfg.Keys.GetKey(fromChain, params.UserAccount, 0)
	if err != nil {
		return 0, fmt.Errorf("Error while getting from key: %v", err)
	}
	fromAddress, err := fromKey.Address(fromChain, cfg.EnvConfig.Network, false)
	if err != nil {
		return 0, fmt.Errorf("Error while getting address string: %v", err)
	}
	toKey, err := cfg.Keys.GetKey(toChain, params.UserAccount, 0)
	if err != nil {
		return 0, fmt.Errorf("Error while getting to key: %v", err)
	}
	toAddress, err := toKey.Address(toChain, cfg.EnvConfig.Network, false)
	if err != nil {
		return 0, fmt.Errorf("Error while getting address string: %v", err)
	}

	id, err := client.CreateOrder(fromAddress, toAddress, params.OrderPair, params.SendAmount, params.ReceiveAmount, secretHash)
	if err != nil {
		return 0, fmt.Errorf("Error while creating order: %v", err)
	}

	if err = userStore.PutSecret(secretHash, hex.EncodeToString(secret[:]), uint64(id)); err != nil {
		return 0, err
	}

	return id, nil
}
