package main

import (
	"os"
	"strconv"
	"time"

	"github.com/catalogfi/cobi/cobid/executor"
	"github.com/catalogfi/cobi/cobid/types"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) != 3 {
		panic("arguments not enough")
	}
	userAccount, err := strconv.ParseUint(os.Args[1], 10, 32)
	if err != nil {
		panic(err)
	}

	isIW, err := strconv.ParseBool(os.Args[2])
	if err != nil {
		panic(err)
	}
	envConfig, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		panic(err)
	}

	var str store.Store
	if envConfig.DB != "" {
		// Initialise db
		str, err = store.NewStore(sqlite.Open(envConfig.DB), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			panic(err)
		}
	} else {
		str, err = store.NewStore(sqlite.Open(utils.DefaultStorePath()), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			panic(err)
		}
	}

	entropy, err := bip39.EntropyFromMnemonic(envConfig.Mnemonic)
	if err != nil {
		panic(err)
	}

	// Load keys
	keys := utils.NewKeys(entropy)

	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	exec := executor.NewExecutor()
	exec.Start(
		types.CoreConfig{
			Logger:    logger,
			EnvConfig: envConfig,
			Keys:      &keys,
			Storage:   str,
		},
		executor.RequestStartExecutor{
			Account:         uint32(userAccount),
			IsInstantWallet: isIW,
		})
}
