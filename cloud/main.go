package main

import (
	"time"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/catalogfi/cobi"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/getsentry/sentry-go"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func main() {
	var cmd = &cobra.Command{
		Use: "COBI - Catalog Order Book clI",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
		Version:           "cloud",
		DisableAutoGenTag: true,
	}

	envConfig, err := utils.LoadExtendedConfig("./config.json")
	if err != nil {
		panic(err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	if envConfig.Sentry != "" {
		client, err := sentry.NewClient(sentry.ClientOptions{Dsn: envConfig.Sentry})
		if err != nil {
			panic(err)
		}
		cfg := zapsentry.Configuration{
			Level: zapcore.ErrorLevel,
		}
		core, err := zapsentry.NewCore(cfg, zapsentry.NewSentryClientFromClient(client))
		if err != nil {
			panic(err)
		}
		logger = zapsentry.AttachCoreToLogger(core, logger)
		defer logger.Sync()
	}

	entropy, err := bip39.EntropyFromMnemonic(envConfig.Mnemonic)
	if err != nil {
		panic(err)
	}

	// Load keys
	keys := utils.NewKeys(entropy)

	// Initialise db
	store, err := store.NewStore(postgres.Open(envConfig.DB), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
		Logger:  glogger.Default.LogMode(glogger.Silent),
	})
	if err != nil {
		panic(err)
	}

	cmd.AddCommand(cobi.Create(envConfig.OrderBook, keys, store, envConfig.Network))
	cmd.AddCommand(cobi.Fill(envConfig.OrderBook, keys, store, envConfig.Network))
	cmd.AddCommand(cobi.Start(envConfig.OrderBook, envConfig.Strategies, keys, store, envConfig.Network, logger, envConfig.DB))
	cmd.AddCommand(cobi.Retry(envConfig.OrderBook, keys, envConfig.Network, store, logger))
	cmd.AddCommand(cobi.Accounts(envConfig.OrderBook, keys, envConfig.Network))
	cmd.AddCommand(cobi.List(envConfig.OrderBook))
	cmd.AddCommand(cobi.Deposit(keys, envConfig.Network, envConfig.DB, logger))
	cmd.AddCommand(cobi.Transfer(envConfig.OrderBook, keys, envConfig.Network, logger, envConfig.DB))
	// cmd.AddCommand(cobi.Network(envConfig.Network, logger))

	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
