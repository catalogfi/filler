package handlers

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/pkg/swapper/bitcoin"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
)

func GetAccounts(cfg types.CoreConfig, params types.RequestAccount) ([]types.AccountInfo, error) {
	if err := types.CheckStrings(params.Asset); err != nil {
		return nil, fmt.Errorf("asset is not valid: %v", err)
	}
	if err := types.CheckUint32s(params.PerPage, params.Page); err != nil {
		return nil, fmt.Errorf("error while parsing PerPage: %v", err)
	}
	ch, a, err := model.ParseChainAsset(params.Asset)
	if err != nil {
		return nil, fmt.Errorf("error while parsing Chain and Asset: %v", err)
	}
	// var iwStore bitcoin.Store
	// var guardianWallet guardian.BitcoinWallet
	// var logger *zap.Logger
	// var chainParams *chaincfg.Params
	// var rpcClient jsonrpc.Client
	// var feeEstimator btc.FeeEstimator
	// var indexer btc.IndexerClient

	var ReturnPayload []types.AccountInfo
	config := cfg.EnvConfig.Network
	iwStore, err := utils.LoadIwDB(cfg.EnvConfig.DB)
	if err != nil {
		return nil, fmt.Errorf("error loading iw db: %v", err)
	}
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("error initializing logger: %v", err)
	}

	// if params.IsInstantWallet {
	// 	iwStore, _ = bitcoin.NewStore(utils.DefaultInstantWalletDBDialector())
	// 	chainParams = blockchain.GetParams(ch)
	// 	rpcClient = jsonrpc.NewClient(new(http.Client), cfg.EnvConfig.Network[ch].IWRPC)
	// 	feeEstimator = btc.NewBlockstreamFeeEstimator(chainParams, cfg.EnvConfig.Network[ch].RPC["mempool"], 20*time.Second)
	// 	indexer = btc.NewElectrsIndexerClient(logger, cfg.EnvConfig.Network[ch].RPC["mempool"], 5*time.Second)

	// }

	for i := params.PerPage*params.Page - params.PerPage; i < params.PerPage*params.Page; i++ {
		key, err := cfg.Keys.GetKey(ch, uint32(i), 0)
		if err != nil {
			return nil, fmt.Errorf("error parsing key: %v", err)
		}

		address, err := key.Address(ch, config, params.IsLegacy)
		if err != nil {
			return nil, fmt.Errorf("error getting wallet address: %v", err)
		}

		signingKey, err := cfg.Keys.GetKey(model.Ethereum, params.UserAccount, uint32(i))
		if err != nil {
			return nil, fmt.Errorf("error getting signing key: %v", err)
		}
		ecdsaKey, err := signingKey.ECDSA()
		if err != nil {
			return nil, fmt.Errorf("error calculating ECDSA key: %v", err)
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
			return nil, fmt.Errorf("failed to calculate evm address: %v", err)
		}

		var balance *big.Int
		var usableBalance *big.Int
		if params.IsInstantWallet {
			guardianWallet, err := utils.GetGuardianWallet(key.BtcKey(), logger, ch, config)
			if err != nil {
				return nil, err
			}

			iwConfig := bitcoin.InstantWalletConfig{
				Store:   iwStore,
				IWallet: guardianWallet,
			}
			address, err = key.Address(ch, config, params.IsLegacy, iwConfig)
			if err != nil {
				return nil, fmt.Errorf("error getting instant wallet address: %v", err)
			}
			balance, err = utils.Balance(ch, address, config, a, iwConfig)
			if err != nil {
				return nil, fmt.Errorf("error getting balance: %v", err)
			}
			usableBalance, err = utils.VirtualBalance(ch, address, config, a, signer.Hex(), client, iwConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get usable balance: %v", err)
			}
		} else {
			balance, err = utils.Balance(ch, address, config, a)
			if err != nil {
				return nil, fmt.Errorf("error getting balance: %v", err)
			}
			usableBalance, err = utils.VirtualBalance(ch, address, config, a, signer.Hex(), client)
			if err != nil {
				return nil, fmt.Errorf("failed to get usable balance: %v", err)
			}
		}

		ReturnPayload = append(ReturnPayload, types.AccountInfo{
			AccountNo:     fmt.Sprintf("%d", i),
			Address:       address,
			Balance:       balance.String(),
			UsableBalance: usableBalance.String(),
		})
	}
	return ReturnPayload, nil
}
