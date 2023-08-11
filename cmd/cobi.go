package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/catalogfi/cobi"
	"github.com/catalogfi/wbtc-garden/model"
)

type Config struct {
	BTC_RPC          string
	ETH_RPC          string
	BTC_TESTNET_RPC  string
	ETH_SEPOLIA_RPC  string
	ETH_OPTIMISM_RPC string
}

func LoadConfiguration(file string) Config {
	var config Config
	configFile, err := os.Open(file)
	if err != nil {
		panic(err)
	}
	defer configFile.Close()
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&config)
	return config
}

func main() {
	envConfig := LoadConfiguration("../../config.json")
	if err := cobi.Run(model.Config{
		RPC: map[model.Chain]string{
			model.Bitcoin:          envConfig.BTC_RPC,
			model.BitcoinTestnet:   envConfig.BTC_TESTNET_RPC,
			model.EthereumSepolia:  envConfig.ETH_SEPOLIA_RPC,
			model.Ethereum:         envConfig.ETH_RPC,
			model.EthereumOptimism: envConfig.ETH_OPTIMISM_RPC,
		},
	}); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
