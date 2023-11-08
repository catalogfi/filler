package handlers

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Transfer(cfg CoreConfig, params RequestTransfer) (string, error) {
	checkStrings(params.Asset, params.ToAddr)
	ch, a, err := model.ParseChainAsset(params.Asset)
	if err != nil {
		return "", (fmt.Errorf("Error while parsing chain and asset: %v", err))
	}

	err = blockchain.CheckAddress(ch, params.ToAddr)
	if err != nil {
		return "", (fmt.Errorf("Invalid address for chain: %s", ch))
	}
	iwConfig := model.InstantWalletConfig{}

	if params.UseIw {

		iwConfig.Dialector = sqlite.Open(cfg.EnvConfig.DB)
		iwConfig.Opts = &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		}
	}

	// defaultIwConfig := utils.GetIWConfig(false)

	key, err := cfg.Keys.GetKey(ch, uint32(params.UserAccount), 0)
	if err != nil {
		return "", (fmt.Errorf("Error while getting the signing key: %v", err))
	}

	iwAddress, err := key.Address(ch, cfg.EnvConfig.Network, iwConfig)
	if err != nil {
		return "", (fmt.Errorf("Error while getting the instant wallet address: %v", err))
	}

	address, err := key.Address(ch, cfg.EnvConfig.Network, utils.GetIWConfig(false))
	if err != nil {
		return "", (fmt.Errorf("Error while getting the address: %v", err))
	}

	signingKey, err := cfg.Keys.GetKey(model.Ethereum, params.UserAccount, 0)
	if err != nil {
		return "", (fmt.Errorf("Error while getting the signing key: %v", err))
	}
	ecdsaKey, err := signingKey.ECDSA()
	if err != nil {
		return "", (fmt.Errorf("Error calculating ECDSA key: %v", err))
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
	amt := new(big.Int).SetUint64(uint64(params.Amount))
	if !params.UseIw {
		balance, err := utils.Balance(ch, address, cfg.EnvConfig.Network, a, iwConfig)
		if err != nil {
			return "", (fmt.Errorf("Error while getting the balance: %v", err))
		}

		if amt.Cmp(balance) > 0 {
			return "", (fmt.Errorf("Amount cannot be greater than balance : %s", balance.String()))
		}
	} else {
		usableBalance, err := utils.VirtualBalance(ch, iwAddress, address, cfg.EnvConfig.Network, a, signer.Hex(), restClient, iwConfig)
		if err != nil {
			return "", (fmt.Errorf("failed to get usable balance: %v", err))
		}
		if amt.Cmp(usableBalance) > 0 && !params.Force {
			return "", (fmt.Errorf("Amount cannot be greater than usable balance : %s", usableBalance.String()))
		}
	}

	client, err := blockchain.LoadClient(ch, cfg.EnvConfig.Network, iwConfig)
	if err != nil {
		return "", (fmt.Errorf("failed to load client: %v", err))
	}

	var txhash string

	switch client := client.(type) {
	case ethereum.Client:
		privKey, err := key.ECDSA()
		if err != nil {
			return "", (fmt.Errorf("Error calculating ECDSA key: %v", err))
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
