package cobi

import (
	"fmt"
	"math/big"
	"time"

	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func Deposit(keys utils.Keys, config model.Network, db string, logger *zap.Logger) *cobra.Command {
	var (
		asset   string
		account uint32
		amount  uint64
	)
	var cmd = &cobra.Command{
		Use:   "deposit",
		Short: "deposit funds from EOA to instant wallets",
		Run: func(c *cobra.Command, args []string) {

			defaultIwStore, _ := bitcoin.NewStore(nil)
			chain, a, err := model.ParseChainAsset(asset)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Error while generating secret: %v", err))
				return
			}
			iwStore, err := bitcoin.NewStore(postgres.Open(db), &gorm.Config{
				NowFunc: func() time.Time { return time.Now().UTC() },
				Logger:  glogger.Default.LogMode(glogger.Silent),
			})
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("Could not load iw store: %v", err))
				return
			}

			client, err := blockchain.LoadClient(chain, config, iwStore)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("failed to load client: %v", err))
				return

			}
			switch client := client.(type) {
			case bitcoin.InstantClient:
				key, err := keys.GetKey(chain, account, 0)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error while getting the signing key: %v", err))
					return
				}

				address, err := key.Address(chain, config, defaultIwStore)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error getting wallet address: %v", err))
					return
				}
				balance, err := utils.Balance(chain, address, config, a, defaultIwStore)
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("Error fetching balance: %v", err))
					return
				}

				if new(big.Int).SetUint64(amount).Cmp(balance) > 0 {
					logger.Info("amount greater than balance", zap.Uint64("amount", amount), zap.Uint64("balance", balance.Uint64()))
					return
				}

				privKey := key.BtcKey()
				txHash, err := client.FundInstantWallet(privKey, int64(amount))
				if err != nil {
					cobra.CheckErr(fmt.Sprintf("failed to deposit to instant wallet: %v", err))
					return

				}
				logger.Info("Bitcoin deposit successful", zap.String("txHash:", txHash))
			}

		}}
	cmd.Flags().Uint32Var(&account, "account", 0, "config file (default: 0)")
	cmd.Flags().Uint64Var(&amount, "amount", 0, "User should provide the amount to deposit to instant wallet")
	cmd.MarkFlagRequired("amount")
	cmd.Flags().StringVarP(&asset, "asset", "a", "", "user should provide the asset")
	cmd.MarkFlagRequired("asset")
	return cmd
}
