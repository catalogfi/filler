package cobi

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
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func Transfer(url string, keys utils.Keys, config model.Network, logger *zap.Logger, db string) *cobra.Command {
	var (
		user   uint32
		amount uint32
		asset  string
		toAddr string
		useIw  bool
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "transfer funds",
		Run: func(c *cobra.Command, args []string) {
			ch, a, err := model.ParseChainAsset(asset)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while generating secret: %v", err))
				return
			}

			err = blockchain.CheckAddress(ch, toAddr)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Invalid address for chain: %s", ch))
			}
			iwConfig := model.InstantWalletConfig{}

			iwConfig.Dialector = postgres.Open(db)
			iwConfig.Opts = &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
				Logger:  glogger.Default.LogMode(glogger.Silent),
			}

			// defaultIwConfig := utils.GetIWConfig(false)

			key, err := keys.GetKey(ch, uint32(user), 0)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error parsing key: %v", err))
				return
			}

			iwAddress, err := key.Address(ch, config, iwConfig)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error getting instant wallet address: %v", err))
				return
			}

			address, err := key.Address(ch, config, utils.GetIWConfig(false))
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error getting instant wallet address: %v", err))
				return
			}

			balance, err := utils.Balance(ch, address, config, a, iwConfig)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error fetching balance: %v", err))
				return
			}

			amt := new(big.Int).SetUint64(uint64(amount))
			if amt.Cmp(balance) > 0 {
				cobra.CheckErr(fmt.Sprintf("Amount cannot be greater than balance : %s", balance.String()))
			}
			signingKey, err := keys.GetKey(model.Ethereum, user, 0)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error getting signing key: %v", err))
				return
			}
			ecdsaKey, err := signingKey.ECDSA()
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error calculating ECDSA key: %v", err))
				return
			}

			restClient := rest.NewClient(fmt.Sprintf("https://%s", url), hex.EncodeToString(crypto.FromECDSA(ecdsaKey)))
			token, err := restClient.Login()
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("failed to get auth token: %v", err))
				return
			}
			if err := restClient.SetJwt(token); err != nil {
				cobra.CheckErr(fmt.Sprintf("failed to set auth token: %v", err))
				return
			}
			signer, err := key.EvmAddress()
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("failed to calculate evm address: %v", err))
				return
			}
			usableBalance, err := utils.VirtualBalance(ch, iwAddress, address, config, a, signer.Hex(), restClient, iwConfig)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("failed to get usable balance: %v", err))
				return
			}
			if amt.Cmp(usableBalance) > 0 && !force {
				cobra.CheckErr(fmt.Sprintf("Amount cannot be greater than usable balance : %s", usableBalance.String()))
			}

			client, err := blockchain.LoadClient(ch, config, iwConfig)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Failed to load client : %v", err))
			}

			var txhash string

			switch client := client.(type) {
			case ethereum.Client:
				privKey, err := key.ECDSA()
				if err != nil {
					cobra.CheckErr(err.Error())
				}

				if a == model.Primary {
					client.TransferEth(privKey, amt, common.HexToAddress(toAddr))
				} else {
					client.TransferERC20(privKey, amt, common.HexToAddress(asset), common.HexToAddress(toAddr))
				}
			case bitcoin.InstantClient:
				toAddress, _ := btcutil.DecodeAddress(toAddr, blockchain.GetParams(ch))
				fmt.Println("iw for sure")
				txhash, err = client.Send(toAddress, uint64(amount), key.BtcKey())
				if err != nil {
					cobra.CheckErr(err.Error())
				}
			case bitcoin.Client:
				toAddress, _ := btcutil.DecodeAddress(toAddr, blockchain.GetParams(ch))
				txhash, err = client.Send(toAddress, uint64(amount), key.BtcKey())
				if err != nil {
					cobra.CheckErr(err.Error())
				}
			}

			logger.Info("send transaction successful", zap.String("txHash :", txhash))

		},
		DisableAutoGenTag: true,
	}
	cmd.Flags().BoolVarP(&useIw, "instant-wallet", "i", false, "user can specify to use catalog instant wallets")
	cmd.Flags().BoolVarP(&force, "force-send", "f", false, "can force send")
	cmd.Flags().StringVarP(&asset, "asset", "a", "", "user should provide the asset")
	cmd.MarkFlagRequired("asset")
	cmd.Flags().StringVar(&toAddr, "toAddress", "", "user should provide the asset")
	cmd.MarkFlagRequired("address")
	cmd.Flags().Uint32Var(&user, "account", 0, "user can provide the user id")
	cmd.Flags().Uint32Var(&amount, "amount", 0, "amount to transfer")
	cmd.MarkFlagRequired("amount")

	return cmd
}
