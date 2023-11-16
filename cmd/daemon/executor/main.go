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

	"github.com/catalogfi/cobi/daemon/executor"
	"github.com/catalogfi/cobi/daemon/types"
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
	userAccount, err := strconv.ParseUint(os.Args[1], 10, 32)
	if err != nil {
		fmt.Fprint(os.Stdout, err)
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

	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(config)
	logFile, _ := os.OpenFile(filepath.Join(utils.DefaultCobiLogs(), fmt.Sprintf("executor_account_%d.log", userAccount)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	writer := zapcore.AddSync(logFile)
	defaultLogLevel := zapcore.DebugLevel
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, defaultLogLevel),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	pidFilePath := filepath.Join(utils.DefaultCobiPids(), fmt.Sprintf("executor_account_%d.pid", userAccount))

	if _, err := os.Stat(pidFilePath); err == nil {
		fmt.Fprint(os.Stdout, "executor already running")
	}
	pid := strconv.Itoa(os.Getpid())
	err = os.WriteFile(pidFilePath, []byte(pid), 0644)
	if err != nil {
		fmt.Fprint(os.Stdout, "failed to write pid")
	}

	wg := new(sync.WaitGroup)

	exec := executor.NewExecutor(types.CoreConfig{
		Logger:    logger,
		EnvConfig: envConfig,
		Keys:      &keys,
		Storage:   str,
	}, wg)

	fmt.Fprint(os.Stdout, "successful")

	wg.Add(1)
	go func() {
		exec.Start(

			executor.RequestStartExecutor{
				Account:         uint32(userAccount),
				IsInstantWallet: isIW,
			})
	}()

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGQUIT)
		<-sigs
		logger.Info("recieved quit signal")
		exec.Done()
		wg.Wait()
	}()

	wg.Wait()

	if _, err := os.Stat(pidFilePath); err == nil {
		err := os.Remove(pidFilePath)
		if err != nil {
			logger.Error("failed to delete executor pid file", zap.Uint32("account", uint32(userAccount)), zap.Error(err))
		}
	} else {
		logger.Error("executor pid file not found", zap.Uint32("account", uint32(userAccount)), zap.Error(err))
	}
	logger.Info("stopped executor", zap.Uint32("account", uint32(userAccount)))

}
