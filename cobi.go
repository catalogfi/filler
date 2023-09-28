package cobi

import (
	"time"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/getsentry/sentry-go"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func Run(version string) error {
	var cmd = &cobra.Command{
		Use: "COBI - Catalog Order Book clI",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
		Version:           version,
		DisableAutoGenTag: true,
	}

	envConfig, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return err
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}
	if envConfig.Sentry != "" {
		client, err := sentry.NewClient(sentry.ClientOptions{Dsn: envConfig.Sentry})
		if err != nil {
			return err
		}
		cfg := zapsentry.Configuration{
			Level: zapcore.ErrorLevel,
		}
		core, err := zapsentry.NewCore(cfg, zapsentry.NewSentryClientFromClient(client))
		if err != nil {
			return err
		}
		logger = zapsentry.AttachCoreToLogger(core, logger)
		defer logger.Sync()
	}

	entropy, err := bip39.EntropyFromMnemonic(envConfig.Mnemonic)
	if err != nil {
		return err
	}

	// Load keys
	keys := utils.NewKeys(entropy)

	var str store.Store
	if envConfig.DB != "" {
		// Initialise db
		str, err = store.NewStore(postgres.Open(envConfig.DB), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
			Logger:  glogger.Default.LogMode(glogger.Silent),
		})
		if err != nil {
			return err
		}
	} else {
		str, err = store.NewStore(sqlite.Open(utils.DefaultStorePath()), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			return err
		}
	}

	cmd.AddCommand(Create(envConfig.OrderBook, keys, str, envConfig.Network))
	cmd.AddCommand(Fill(envConfig.OrderBook, keys, str, envConfig.Network))
	cmd.AddCommand(Start(envConfig.OrderBook, envConfig.Strategies, keys, str, envConfig.Network, logger, envConfig.DB))
	cmd.AddCommand(Retry(envConfig.OrderBook, keys, envConfig.Network, str, logger, envConfig.DB))
	cmd.AddCommand(Accounts(envConfig.OrderBook, keys, envConfig.Network))
	cmd.AddCommand(List(envConfig.OrderBook))
	// cmd.AddCommand(Network(envConfig.Network, logger))
	// cmd.AddCommand(Update())
	cmd.AddCommand(Deposit(keys, envConfig.Network, envConfig.DB, logger))
	cmd.AddCommand(Transfer(envConfig.OrderBook, keys, envConfig.Network, logger, envConfig.DB))
	if err := cmd.Execute(); err != nil {
		return err
	}
	return nil
}
