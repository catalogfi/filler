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
		fmt.Fprint(os.Stdout, "arguments not enough")
		return
	}

	serviceType := os.Args[1]

	var isCreator, isFiller bool
	switch serviceType {
	case "autofiller":
		isFiller = true
	case "autocreator":
		isCreator = true
	default:
		fmt.Fprint(os.Stdout, "not a valid service")
		return

	}

	isIW, err := strconv.ParseBool(os.Args[2])
	if err != nil {
		fmt.Fprint(os.Stdout, err)
		return
	}

	envConfig, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		fmt.Fprint(os.Stdout, err)
		return
	}

	var str store.Store
	if envConfig.DB != "" {
		// Initialise db
		str, err = store.NewStore(sqlite.Open(envConfig.DB), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			fmt.Fprint(os.Stdout, err)
			return
		}
	} else {
		str, err = store.NewStore(sqlite.Open(utils.DefaultStorePath()), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			fmt.Fprint(os.Stdout, err)
			return
		}
	}

	entropy, err := bip39.EntropyFromMnemonic(envConfig.Mnemonic)
	if err != nil {
		fmt.Fprint(os.Stdout, err)
		return
	}

	// Load keys
	keys := utils.NewKeys(entropy)

	loggerConfig := zap.NewProductionEncoderConfig()
	loggerConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(loggerConfig)
	logFile, _ := os.OpenFile(filepath.Join(utils.DefaultCobiLogs(), fmt.Sprintf("%s.log", serviceType)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	writer := zapcore.AddSync(logFile)
	defaultLogLevel := zapcore.DebugLevel
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, defaultLogLevel),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	pidFilePath := filepath.Join(utils.DefaultCobiPids(), fmt.Sprintf("%s.pid", serviceType))

	if _, err := os.Stat(pidFilePath); err == nil {
		fmt.Fprintf(os.Stdout, "%s already running", serviceType)
		return
	}
	pid := strconv.Itoa(os.Getpid())

	err = os.WriteFile(pidFilePath, []byte(pid), 0644)
	if err != nil {
		fmt.Fprintf(os.Stdout, "failed to write pid")
		return
	}

	config := types.CoreConfig{
		Logger:    logger,
		EnvConfig: envConfig,
		Keys:      &keys,
		Storage:   str,
	}

	strategies, err := strategy.UnmarshalStrategy(envConfig.Strategies)
	if err != nil {
		fmt.Fprintf(os.Stdout, "failed to unmarshal strategy, err:%v", err)
		return
	}

	wg := new(sync.WaitGroup)

	strat := strategy.NewStrategy(config, wg)

	if isCreator {
		for _, s := range strategies {
			switch service := s.(type) {
			case strategy.AutoCreateStrategy:
				if _, err := os.Stat(filepath.Join(utils.DefaultCobiPids(), fmt.Sprintf("executor_account_%d.pid", service.Account()))); err != nil {
					fmt.Fprintf(os.Stdout, "executor not running, account:%d", service.Account())
					strat.Done()
					wg.Wait()
					return
				}
				wg.Add(1)
				go strat.RunAutoCreateStrategy(service, isIW)
			}
		}
	} else if isFiller {
		for _, s := range strategies {
			switch service := s.(type) {
			case strategy.AutoFillStrategy:
				if _, err := os.Stat(filepath.Join(utils.DefaultCobiPids(), fmt.Sprintf("executor_account_%d.pid", service.Account()))); err != nil {
					fmt.Fprintf(os.Stdout, "executor not running, account:%d", service.Account())
					strat.Done()
					wg.Wait()
					return
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

	fmt.Fprintf(os.Stdout, "successful")

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
