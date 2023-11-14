package main

import (
	"time"

	jsonrpc "github.com/catalogfi/cobi/cobid/rpc"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	// start executor
	// start autofillers
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
	rpcServer := jsonrpc.NewRpcServer(str, envConfig, &keys, logger)
	rpcServer.Run()
}
