package handlers

import (
	"encoding/hex"
	"fmt"

	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/crypto"
)

type AccountInfo struct {
	AccountNo     string `json:"accountNo"`
	Address       string `json:"address"`
	Balance       string `json:"balance"`
	UsableBalance string `json:"usableBalance"`
}

func GetAccounts(cfg CoreConfig, params RequestAccount) ([]AccountInfo, error) {
	if err := checkStrings(params.Asset); err != nil {
		return nil, fmt.Errorf("Asset is not valid: %v", err)
	}
	ch, a, err := model.ParseChainAsset(params.Asset)
	if err != nil {
		return nil, fmt.Errorf("Error while parsing Chain and Asset: %v", err)
	}

	iwConfig := utils.GetIWConfig(params.IsInstantWallet)
	defaultIwConfig := utils.GetIWConfig(false)

	config := cfg.EnvConfig.Network

	var ReturnPayload []AccountInfo

	for i := params.PerPage*params.Page - params.PerPage; i < params.PerPage*params.Page; i++ {
		key, err := cfg.Keys.GetKey(ch, uint32(i), 0)
		if err != nil {
			return nil, fmt.Errorf("Error parsing key: %v", err)
		}

		iwAddress, err := key.Address(ch, config, iwConfig)
		if err != nil {
			return nil, fmt.Errorf("Error while getting the instant wallet address: %v", err)
		}

		address, err := key.Address(ch, config, defaultIwConfig)
		if err != nil {
			return nil, fmt.Errorf("Error while getting the wallet address: %v", err)
		}
		balance, err := utils.Balance(ch, iwAddress, config, a, iwConfig)
		if err != nil {
			return nil, fmt.Errorf("Error while getting the balance: %v", err)
		}

		signingKey, err := cfg.Keys.GetKey(model.Ethereum, params.UserAccount, uint32(i))
		if err != nil {
			return nil, fmt.Errorf("Error while getting the signing key: %v", err)
		}
		ecdsaKey, err := signingKey.ECDSA()
		if err != nil {
			return nil, fmt.Errorf("Error calculating ECDSA key: %v", err)
		}

		client := rest.NewClient(fmt.Sprintf("https://%s", cfg.EnvConfig.OrderBook), hex.EncodeToString(crypto.FromECDSA(ecdsaKey)))
		token, err := client.Login()
		if err != nil {
			return nil, fmt.Errorf("failed to get auth token: %v", err)
		}
		if err := client.SetJwt(token); err != nil {
			return nil, fmt.Errorf("failed to set auth token: %v", err)
		}

		signer, err := key.EvmAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to get signer address: %v", err)
		}

		usableBalance, err := utils.VirtualBalance(ch, iwAddress, address, config, a, signer.Hex(), client, iwConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to get usable balance: %v", err)
		}
		if params.IsInstantWallet {
			address, err = key.Address(ch, config, iwConfig)
			if err != nil {
				return nil, fmt.Errorf("Error while getting the Instant wallet address: %v", err)
			}
		}

		ReturnPayload = append(ReturnPayload, AccountInfo{
			AccountNo:     fmt.Sprintf("%d", i),
			Address:       address,
			Balance:       balance.String(),
			UsableBalance: usableBalance.String(),
		})
	}
	return ReturnPayload, nil
}
