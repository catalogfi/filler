package handlers

import (
	"fmt"
	"math/big"
	"time"

	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Deposit(cfg CoreConfig, params RequestDeposit) (string, error) {
	if err := checkStrings(params.Asset); err != nil {
		return "", fmt.Errorf("Asset is not valid: %v", err)
	}
	if err := checkUint64s((params.Amount)); err != nil {
		return "", fmt.Errorf("Amount is not valid: %v", err)
	}
	defaultIwStore, _ := bitcoin.NewStore(nil)
	chain, a, err := model.ParseChainAsset(params.Asset)
	if err != nil {
		return "", fmt.Errorf("Error while parsing chain asset: %v", err)
	}
	iwStore, err := bitcoin.NewStore(postgres.Open(cfg.EnvConfig.DB), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return "", fmt.Errorf("Could not load iw store: %v", err)
	}

	client, err := blockchain.LoadClient(chain, cfg.EnvConfig.Network, iwStore)
	if err != nil {
		return "", fmt.Errorf("failed to load client: %v", err)
	}
	switch client := client.(type) {
	case bitcoin.InstantClient:
		key, err := cfg.Keys.GetKey(chain, params.UserAccount, 0)
		if err != nil {
			return "", fmt.Errorf("Error while getting the signing key: %v", err)
		}

		address, err := key.Address(chain, cfg.EnvConfig.Network, defaultIwStore)
		if err != nil {
			return "", fmt.Errorf("Error getting wallet address: %v", err)
		}
		balance, err := utils.Balance(chain, address, cfg.EnvConfig.Network, a, defaultIwStore)
		if err != nil {
			return "", fmt.Errorf("Error getting wallet balance: %v", err)
		}

		if new(big.Int).SetUint64(params.Amount).Cmp(balance) > 0 {
			return "", fmt.Errorf("Insufficient funds")
		}

		privKey := key.BtcKey()
		txHash, err := client.FundInstantWallet(privKey, int64(params.Amount))
		if err != nil {
			return "", fmt.Errorf("Error funding wallet: %v", err)

		}
		return txHash, nil
	}
	return "", fmt.Errorf("Invalid Asset Passet: %s", params.Asset)
}
