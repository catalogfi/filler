package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/catalogfi/cobi/daemon/strategy"
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
	strategyUid := os.Args[1]

	isIW, err := strconv.ParseBool(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stdout, "err : %v", err)
		return
	}

	// Load config
	envConfig, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stdout, "err : %v", err)
		return
	}

	stratBytes, err := json.MarshalIndent(envConfig.Strategies, "", " ")
	if err != nil {
		fmt.Fprintf(os.Stdout, "failed to unmarshal strategy, err:%v", err)
		return
	}

	strategies := []strategy.Strategy{}
	if err := json.Unmarshal(stratBytes, &strategies); err != nil {
		fmt.Fprintf(os.Stdout, "failed to unmarshal strategy, err:%v", err)
		return
	}

	strat, err := searchStrat(strategyUid, strategies)
	if err != nil {
		fmt.Fprintf(os.Stdout, "err: %v", err)
		return
	}

	// Initialise db
	var str store.Store
	if envConfig.DB != "" {
		str, err = store.NewStore(sqlite.Open(envConfig.DB), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			fmt.Fprintf(os.Stdout, "err : %v", err)
			return
		}
	} else {
		str, err = store.NewStore(sqlite.Open(utils.DefaultStorePath()), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			fmt.Fprintf(os.Stdout, "err : %v", err)
			return
		}
	}

	entropy, err := bip39.EntropyFromMnemonic(envConfig.Mnemonic)
	if err != nil {
		fmt.Fprintf(os.Stdout, "err : %v", err)
		return
	}

	// Load keys
	keys := utils.NewKeys(entropy)

	// Calculate uid
	uid, err := strategy.Uid(strat)
	if err != nil {
		fmt.Fprint(os.Stdout, err)
		return
	}

	// Configure logger
	logger := process.NewFileLogger(uid)

	// Initialize config
	config := types.CoreConfig{
		Logger:    logger,
		EnvConfig: envConfig,
		Keys:      &keys,
		Storage:   str,
	}
	wg := new(sync.WaitGroup)

	// Initialize strategy
	s := strategy.NewStrategyService(config, wg)

	// Initialize PID manager
	pidManager := process.NewPidManager(uid)

	// Strart service
	seriveType := strings.Split(strat.StrategyType, "-")
	if len(seriveType) < 2 {
		fmt.Fprintf(os.Stdout, "invalid strat type")
		return
	}
	switch strategy.StrategyType(seriveType[1]) {
	case strategy.Filler:
		{
			service, err := strategy.StrategyToAutoFillStrategy(strat)
			if err != nil {
				fmt.Fprintf(os.Stdout, "err : %v", err)
				return
			}
			wg.Add(1)
			go s.RunAutoFillStrategy(service, isIW)
		}
	case strategy.Creator:
		{
			wg.Add(1)
			service, err := strategy.StrategyToAutoCreateStrategy(strat)
			if err != nil {
				fmt.Fprintf(os.Stdout, "err : %v", err)
				return
			}
			go s.RunAutoCreateStrategy(service, isIW)
		}
	}

	// Start signal receiver
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGQUIT)
		<-sigs
		s.Done()
		wg.Wait()
	}()

	// Create pid file
	err = pidManager.Write()
	if err != nil {
		fmt.Fprintf(os.Stdout, "err : %v", err)
		return
	}

	// Write success to pipe
	fmt.Fprint(os.Stdout, process.DefaultSuccessfulMsg)

	wg.Wait()

	// Remove pid file
	err = pidManager.Remove()
	if err != nil {
		logger.Error("failed to delete pid file", zap.String("service :", strat.StrategyType), zap.Error(err))
		return
	}

	logger.Info("stopped", zap.String("service : ", strat.StrategyType))

}

func searchStrat(strategyUid string, strategies []strategy.Strategy) (strategy.Strategy, error) {
	for _, s := range strategies {
		uid, err := strategy.Uid(s)
		if err != nil {
			return strategy.Strategy{}, fmt.Errorf("%v", err)
		}
		if strings.Compare(uid, strategyUid) == 0 {
			return s, nil
		}
	}

	return strategy.Strategy{}, fmt.Errorf("strategy not found")

}
