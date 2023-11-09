package handlers

import (
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"github.com/catalogfi/guardian"
	"github.com/catalogfi/guardian/jsonrpc"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Deposit(cfg CoreConfig, params RequestDeposit) (string, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return "", err
	}
	if err := checkStrings(params.Asset); err != nil {
		return "", fmt.Errorf("Asset is not valid: %v", err)
	}
	if err := checkUint64s((params.Amount)); err != nil {
		return "", fmt.Errorf("Amount is not valid: %v", err)
	}
	chain, a, err := model.ParseChainAsset(params.Asset)
	if err != nil {
		return "", fmt.Errorf("Error while parsing chain asset: %v", err)
	}

	var iwStore bitcoin.Store
	if cfg.EnvConfig.DB != "" {

		iwStore, err = bitcoin.NewStore(sqlite.Open(cfg.EnvConfig.DB), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			return "", fmt.Errorf("Could not load iw store: %v", err)
		}
	} else {
		iwStore, err = bitcoin.NewStore((utils.DefaultInstantWalletDBDialector()), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			return "", fmt.Errorf("Could not load iw store: %v", err)
		}
	}

	key, err := cfg.Keys.GetKey(chain, params.UserAccount, 0)
	if err != nil {
		return "", fmt.Errorf("Error while getting the signing key: %v", err)
	}

	privKey := key.BtcKey()
	chainParams := blockchain.GetParams(chain)
	rpcClient := jsonrpc.NewClient(new(http.Client), cfg.EnvConfig.Network[chain].IWRPC)
	feeEstimator := btc.NewBlockstreamFeeEstimator(chainParams, cfg.EnvConfig.Network[chain].RPC["mempool"], 20*time.Second)
	indexer := btc.NewElectrsIndexerClient(logger, cfg.EnvConfig.Network[chain].RPC["mempool"], 5*time.Second)

	guardianWallet, err := guardian.NewBitcoinWallet(logger, privKey, chainParams, indexer, feeEstimator, rpcClient)

	iwConfig := bitcoin.InstantWalletConfig{
		Store:   iwStore,
		IWallet: guardianWallet,
	}
	if err != nil {
		return "", fmt.Errorf("Could not load iw store: %v", err)
	}

	client, err := blockchain.LoadClient(chain, cfg.EnvConfig.Network, iwConfig)
	if err != nil {
		return "", fmt.Errorf("failed to load client: %v", err)
	}
	switch client := client.(type) {
	case bitcoin.InstantClient:

		address, err := key.Address(chain, cfg.EnvConfig.Network, false)
		if err != nil {
			return "", fmt.Errorf("Error getting wallet address: %v", err)
		}
		balance, err := utils.Balance(chain, address, cfg.EnvConfig.Network, a)
		if err != nil {
			return "", fmt.Errorf("Error getting wallet balance: %v", err)
		}

		if new(big.Int).SetUint64(params.Amount).Cmp(balance) > 0 {
			return "", fmt.Errorf("Insufficient funds")
		}

		txHash, err := client.FundInstantWallet(privKey, int64(params.Amount))
		if err != nil {
			return "", fmt.Errorf("Error funding wallet: %v", err)

		}
		return txHash, nil
	}
	return "", fmt.Errorf("Invalid Asset Passet: %s", params.Asset)
}
