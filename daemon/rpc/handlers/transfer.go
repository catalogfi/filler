package handlers

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/pkg/blockchain"
	"github.com/catalogfi/cobi/pkg/swapper/bitcoin"
	"github.com/catalogfi/cobi/pkg/swapper/ethereum"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/guardian"
	"github.com/catalogfi/guardian/jsonrpc"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/rest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Transfer(cfg types.CoreConfig, params types.RequestTransfer) (string, error) {
	err := types.CheckStrings(params.Asset, params.ToAddr)
	if err != nil {
		return "", (fmt.Errorf("error while parsing asset and address: %v", err))
	}

	if err := types.CheckUint64s(params.Amount); err != nil {
		return "", fmt.Errorf("error while parsing amount: %v", err)
	}

	ch, a, err := model.ParseChainAsset(params.Asset)
	if err != nil {
		return "", (fmt.Errorf("error while parsing chain and asset: %v", err))
	}

	err = blockchain.CheckAddress(ch, params.ToAddr)
	if err != nil {
		return "", (fmt.Errorf("Invalid address for chain: %s", ch))
	}

	key, err := cfg.Keys.GetKey(ch, uint32(params.UserAccount), 0)
	if err != nil {
		return "", (fmt.Errorf("error while getting the signing key: %v", err))
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return "", err
	}

	signingKey, err := cfg.Keys.GetKey(model.Ethereum, params.UserAccount, 0)
	if err != nil {
		return "", (fmt.Errorf("error while getting the signing key: %v", err))
	}
	ecdsaKey, err := signingKey.ECDSA()
	if err != nil {
		return "", (fmt.Errorf("error calculating ECDSA key: %v", err))
	}

	restClient := rest.NewClient(fmt.Sprintf("https://%s", cfg.EnvConfig.OrderBook), hex.EncodeToString(crypto.FromECDSA(ecdsaKey)))
	token, err := restClient.Login()
	if err != nil {
		return "", (fmt.Errorf("failed to get auth token: %v", err))
	}
	if err := restClient.SetJwt(token); err != nil {
		return "", (fmt.Errorf("failed to set auth token: %v", err))
	}
	signer, err := key.EvmAddress()
	if err != nil {
		return "", (fmt.Errorf("failed to get signer address: %v", err))
	}

	var iwConfig bitcoin.InstantWalletConfig

	var address string

	var client interface{}

	amt := new(big.Int).SetUint64(uint64(params.Amount))

	if params.UseIw {
		var iwStore bitcoin.Store
		if cfg.EnvConfig.DB != "" {
			iwStore, err = bitcoin.NewStore(postgres.Open(cfg.EnvConfig.DB), &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
			})
			if err != nil {
				return "", (fmt.Errorf("could not load iw store: %s", ch))
			}
		} else {
			iwStore, err = bitcoin.NewStore(utils.DefaultInstantWalletDBDialector(), &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
			})
			if err != nil {
				return "", (fmt.Errorf("could not load iw store: %s", ch))
			}
		}
		privKey := key.BtcKey()
		chainParams := blockchain.GetParams(ch)
		rpcClient := jsonrpc.NewClient(new(http.Client), cfg.EnvConfig.Network[ch].IWRPC)
		feeEstimator := btc.NewBlockstreamFeeEstimator(chainParams, cfg.EnvConfig.Network[ch].RPC["mempool"], 20*time.Second)
		indexer := btc.NewElectrsIndexerClient(logger, cfg.EnvConfig.Network[ch].RPC["mempool"], 5*time.Second)

		guardianWallet, err := guardian.NewBitcoinWallet(logger, privKey, chainParams, indexer, feeEstimator, rpcClient)
		if err != nil {
			return "", err
		}
		iwConfig = bitcoin.InstantWalletConfig{
			Store:   iwStore,
			IWallet: guardianWallet,
		}
		address, err = key.Address(ch, cfg.EnvConfig.Network, false, iwConfig)
		if err != nil {
			return "", (fmt.Errorf("error while getting the instant wallet address: %v", err))
		}
		usableBalance, err := utils.VirtualBalance(ch, address, cfg.EnvConfig.Network, a, signer.Hex(), restClient, iwConfig)
		if err != nil {
			return "", (fmt.Errorf("failed to get usable balance: %v", err))
		}
		if amt.Cmp(usableBalance) > 0 && !params.Force {
			return "", (fmt.Errorf("Amount cannot be greater than usable balance : %s", usableBalance.String()))
		}
		client, err = blockchain.LoadClient(ch, cfg.EnvConfig.Network, iwConfig)
		if err != nil {
			return "", (fmt.Errorf("failed to load client: %v", err))
		}
	} else {
		address, err = key.Address(ch, cfg.EnvConfig.Network, false)
		if err != nil {
			return "", (fmt.Errorf("error while getting the wallet address: %v", err))
		}
		usableBalance, err := utils.VirtualBalance(ch, address, cfg.EnvConfig.Network, a, signer.Hex(), restClient)
		if err != nil {
			return "", (fmt.Errorf("failed to get usable balance: %v", err))
		}
		if amt.Cmp(usableBalance) > 0 && !params.Force {
			return "", (fmt.Errorf("Amount cannot be greater than usable balance : %s", usableBalance.String()))
		}
		client, err = blockchain.LoadClient(ch, cfg.EnvConfig.Network)
		if err != nil {
			return "", (fmt.Errorf("failed to load client: %v", err))
		}
	}

	var txhash string

	switch client := client.(type) {
	case ethereum.Client:
		privKey, err := key.ECDSA()
		if err != nil {
			return "", (fmt.Errorf("error calculating ECDSA key: %v", err))
		}

		if a == model.Primary {
			client.TransferEth(privKey, amt, common.HexToAddress(params.ToAddr))
		} else {
			tokenAddress, err := client.GetTokenAddress(common.HexToAddress(string(a)))
			if err != nil {
				return "", (fmt.Errorf("failed to get token address: %v", err))
			}
			client.TransferERC20(privKey, amt, tokenAddress, common.HexToAddress(params.ToAddr))
		}
	case bitcoin.InstantClient:
		toAddress, _ := btcutil.DecodeAddress(params.ToAddr, blockchain.GetParams(ch))
		txhash, err = client.Send(toAddress, uint64(params.Amount), key.BtcKey())
		if err != nil {
			return "", (fmt.Errorf("failed to send transaction: %v", err))
		}
	case bitcoin.Client:
		toAddress, _ := btcutil.DecodeAddress(params.ToAddr, blockchain.GetParams(ch))
		txhash, err = client.Send(toAddress, uint64(params.Amount), key.BtcKey())
		if err != nil {
			return "", (fmt.Errorf("failed to send transaction: %v", err))
		}
	}

	return txhash, nil
}
