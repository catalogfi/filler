package main

import (
	"os"
	"strconv"
	"time"

	"github.com/catalogfi/cobi/cobid/strategy"
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

	config := types.CoreConfig{
		Logger:    logger,
		EnvConfig: envConfig,
		Keys:      &keys,
		Storage:   str,
	}

	strategies, err := strategy.UnmarshalStrategy(envConfig.Strategies)
	if err != nil {
		logger.Error("failed to unmarshal strategy", zap.Error(err))
		return
	}

	strat := strategy.NewStrategy(config)

	var isAuto, isCreator, isFiller bool

	switch os.Args[1] {
	case "autofiller":
		isFiller = true
	case "autocreator":
		isCreator = true
	case "auto":
		isAuto = true
	}

	for _, s := range strategies {
		switch service := s.(type) {
		case strategy.AutoCreateStrategy:
			if isAuto || isCreator {
				go strat.RunAutoCreateStrategy(service, isIW)
			}
		case strategy.AutoFillStrategy:
			if isAuto || isFiller {
				go strat.RunAutoFillStrategy(service, isIW)
			}

		}
	}

}
