package main

import (
	"context"
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/catalogfi/cobi/wbtc-garden/screener"
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

	config := model.Network{
		"bitcoin_testnet": model.NetworkConfig{
			Assets: map[model.Asset]model.Token{
				model.Primary: {
					Oracle:   "https://api.coincap.io/v2/assets/bitcoin",
					Decimals: 8,
				},
			},
			RPC:    map[string]string{"mempool": "https://mempool.space/testnet/api"},
			Expiry: 0},
		"ethereum_sepolia": model.NetworkConfig{
			Assets: map[model.Asset]model.Token{
				model.NewSecondary("0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF"): {
					Oracle:       "https://api.coincap.io/v2/assets/bitcoin",
					TokenAddress: "0x130Ff59B75a415d0bcCc2e996acAf27ce70fD5eF",
					Decimals:     8,
				}},
			RPC:    map[string]string{"ethrpc": "https://gateway.tenderly.co/public/sepolia"},
			Expiry: 0},
	}

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	watcher := watcher.NewWatcher(logger, store, config, 4)
	go watcher.Run(context.Background())

	// Screen is not doing sanction check in this case
	screener := screener.NewScreener(nil, "")
	server := rest.NewServer(store, model.Config{Network: config}, logger, "SECRET", screener)
	if err := server.Run(context.Background(), ":8080"); err != nil {
		panic(err)
	}
}
