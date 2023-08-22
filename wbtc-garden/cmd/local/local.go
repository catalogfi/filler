package main

import (
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/price"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/catalogfi/cobi/wbtc-garden/store"
	"github.com/catalogfi/cobi/wbtc-garden/watcher"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	// sqlite db
	store, err := store.New(sqlite.Open("test.db"), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		panic(err)
	}

	config := model.Config{
		model.BitcoinTestnet: {
			RPC: "https://mempool.space/testnet/api",
			Assets: map[model.Asset]bool{
				model.Primary: true,
			},
			Expiry: 144,
		},
		model.EthereumSepolia: {
			RPC: "https://gateway.tenderly.co/public/sepolia",
			Assets: map[model.Asset]bool{
				model.NewSecondary(""): true,
			},
			Expiry: 6542,
		},
		model.EthereumOptimism: {
			RPC: "https://opt-mainnet.g.alchemy.com/v2/lM_wORHU7fDVp_SSYJPCCO-erSffgpX9",
			Assets: map[model.Asset]bool{
				model.NewSecondary(""): true,
			},
			Expiry: 10000,
		},
	}

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	watcher := watcher.NewWatcher(logger, store, config)
	price := price.NewPriceChecker(store, "https://api.coincap.io/v2/assets/bitcoin")
	go price.Run()
	go watcher.Run()
	server := rest.NewServer(store, config, logger, "SECRET")
	if err := server.Run(":8080"); err != nil {
		panic(err)
	}
}
