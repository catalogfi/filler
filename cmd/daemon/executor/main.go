package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/catalogfi/cobi/daemon/executor"
	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/pkg/process"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprint(os.Stdout, "arguments not enough")
		return
	}

	// Format inputs
	userAccount, err := strconv.ParseUint(os.Args[1], 10, 16)
	if err != nil {
		fmt.Fprint(os.Stdout, err)
		return
	}

	isIW, err := strconv.ParseBool(os.Args[2])
	if err != nil {
		fmt.Fprint(os.Stdout, err)
		return
	}

	// Load config
	envConfig, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		fmt.Fprint(os.Stdout, err)
		return
	}

	// Initialize db
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

	uid, err := executor.Uid(uint32(userAccount))
	if err != nil {
		fmt.Fprint(os.Stdout, err)
		return
	}

	logger := process.NewFileLogger(uid)

	// Initialize PID manager
	pidManager := process.NewPidManager(uid)

	// Initialize config
	config := types.CoreConfig{
		Logger:    logger,
		EnvConfig: envConfig,
		Keys:      &keys,
		Storage:   str,
	}
	wg := new(sync.WaitGroup)

	exec := executor.NewExecutor(config, wg)

	// Start service
	wg.Add(1)
	go func() {
		exec.Start(

			executor.RequestStartExecutor{
				Account:         uint32(userAccount),
				IsInstantWallet: isIW,
			})
	}()

	// Start signal receiver
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGQUIT)
		<-sigs
		logger.Info("recieved quit signal")
		exec.Done()
		wg.Wait()
	}()

	// Create pid file
	err = pidManager.Write()
	if err != nil {
		fmt.Fprintf(os.Stdout, "err : %v", err)
		return
	}

	fmt.Fprint(os.Stdout, process.DefaultSuccessfulMsg)

	wg.Wait()

	// Remove pid file
	err = pidManager.Remove()
	if err != nil {
		logger.Error("failed to delete executor pid file", zap.Uint32("account", uint32(userAccount)), zap.Error(err))
		return
	}

	logger.Info("stopped executor", zap.Uint32("account", uint32(userAccount)))

}
