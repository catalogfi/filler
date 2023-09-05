package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/store"
	"github.com/catalogfi/cobi/wbtc-garden/watcher"
	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	SENTRY_DSN string
	PSQL_DB    string        `binding:"required"`
	CONFIG     model.Network `binding:"required"`
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
	env := LoadConfiguration("./config.json")
	store, err := store.New(postgres.Open(env.PSQL_DB), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		panic(err)
	}

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	if env.SENTRY_DSN != "" {
		client, err := sentry.NewClient(sentry.ClientOptions{Dsn: env.SENTRY_DSN})
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

	watcher := watcher.NewWatcher(logger, store, env.CONFIG, 4)
	watcher.Run(context.Background())
}
