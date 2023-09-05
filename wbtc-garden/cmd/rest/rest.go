package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest"
	"github.com/catalogfi/cobi/wbtc-garden/screener"
	"github.com/catalogfi/cobi/wbtc-garden/store"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	PORT          string       `binding:"required"`
	PSQL_DB       string       `binding:"required"`
	SERVER_SECRET string       `binding:"required"`
	CONFIG        model.Config `binding:"required"`
	TRM_KEY       string
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
	// psql db
	envConfig := LoadConfiguration("./config.json")
	// fmt.Println(envConfig.PSQL_DB)
	store, err := store.New(postgres.Open(envConfig.PSQL_DB), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		panic(err)
	}

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	screener := screener.NewScreener(store.Gorm(), envConfig.TRM_KEY)
	server := rest.NewServer(store, envConfig.CONFIG, logger, envConfig.SERVER_SECRET, screener)
	if err := server.Run(context.Background(), fmt.Sprintf(":%s", envConfig.PORT)); err != nil {
		panic(err)
	}
}
