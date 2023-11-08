package handlers

import (
	"fmt"
	"math/big"
	"time"

	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Deposit(cfg CoreConfig, params RequestDeposit) (string, error) {
	checkStrings(params.Asset)
	defaultIwConfig := utils.GetIWConfig(false)
	chain, a, err := model.ParseChainAsset(params.Asset)
	if err != nil {
		return "", fmt.Errorf("Error while parsing chain asset: %v", err)
	}
	iwConfig := model.InstantWalletConfig{}

	iwConfig.Dialector = sqlite.Open(cfg.EnvConfig.DB)
	iwConfig.Opts = &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
	}

	client, err := blockchain.LoadClient(chain, cfg.EnvConfig.Network, iwConfig)
	if err != nil {
		return "", fmt.Errorf("failed to load client: %v", err)
	}
	switch client := client.(type) {
	case bitcoin.InstantClient:
		key, err := cfg.Keys.GetKey(chain, params.UserAccount, 0)
		if err != nil {
			return "", fmt.Errorf("Error while getting the signing key: %v", err)
		}

		address, err := key.Address(chain, cfg.EnvConfig.Network, defaultIwConfig)
		if err != nil {
			return "", fmt.Errorf("Error getting wallet address: %v", err)
		}
		balance, err := utils.Balance(chain, address, cfg.EnvConfig.Network, a, defaultIwConfig)
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
