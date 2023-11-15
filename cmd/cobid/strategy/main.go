package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/catalogfi/cobi/cobid/strategy"
	"github.com/catalogfi/cobi/cobid/types"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) != 3 {
		panic("arguments not enough")
	}

	serviceType := os.Args[1]

	var isCreator, isFiller bool
	switch serviceType {
	case "autofiller":
		isFiller = true
	case "autocreator":
		isCreator = true
	default:
		panic("not a valid service")

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

	loggerConfig := zap.NewProductionEncoderConfig()
	loggerConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(loggerConfig)
	logFile, _ := os.OpenFile(filepath.Join(utils.DefaultCobiDirectory(), fmt.Sprintf("%s.log", serviceType)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	writer := zapcore.AddSync(logFile)
	defaultLogLevel := zapcore.DebugLevel
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, defaultLogLevel),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	pidFilePath := filepath.Join(utils.DefaultCobiDirectory(), fmt.Sprintf("%s.pid", serviceType))

	if _, err := os.Stat(pidFilePath); err == nil {
		panic("executor already running")
	}
	pid := strconv.Itoa(os.Getpid())

	err = os.WriteFile(pidFilePath, []byte(pid), 0644)
	if err != nil {
		panic("failed to write pid")
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

	wg := new(sync.WaitGroup)

	strat := strategy.NewStrategy(config, wg)

	if isCreator {
		for _, s := range strategies {
			switch service := s.(type) {
			case strategy.AutoCreateStrategy:
				if _, err := os.Stat(filepath.Join(utils.DefaultCobiDirectory(), fmt.Sprintf("executor_account_%d.pid", service.Account()))); err != nil {
					//send mssage to rpc
					continue
				}
				wg.Add(1)
				go strat.RunAutoCreateStrategy(service, isIW)
			}
		}
	} else if isFiller {
		for _, s := range strategies {
			switch service := s.(type) {
			case strategy.AutoFillStrategy:
				if _, err := os.Stat(filepath.Join(utils.DefaultCobiDirectory(), fmt.Sprintf("executor_account_%d.pid", service.Account()))); err != nil {
					//send mssage to rpc
					continue
				}
				wg.Add(1)
				go strat.RunAutoFillStrategy(service, isIW)
			}
		}
	}

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGQUIT)
		<-sigs
		strat.Done()
		wg.Wait()
	}()

	wg.Wait()

	if _, err := os.Stat(pidFilePath); err == nil {
		err := os.Remove(pidFilePath)
		if err != nil {
			logger.Error("failed to delete pid file", zap.String("service :", serviceType), zap.Error(err))
		}
	} else {
		logger.Error("pid file not found", zap.String("service :", serviceType), zap.Error(err))
	}
	logger.Info("stopped", zap.String("service : ", serviceType))

}
