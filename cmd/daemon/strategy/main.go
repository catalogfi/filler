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

	"github.com/catalogfi/cobi/cmd/daemon/common"
	"github.com/catalogfi/cobi/daemon/strategy"
	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/pkg/process"
	"github.com/catalogfi/cobi/utils"
	"go.uber.org/zap"
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
		fmt.Fprintf(os.Stdout, "failed to parse isIw ,%v", err)
		return
	}

	// Load config
	envConfig, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stdout, "failed to load config, %v", err)
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
		fmt.Fprintf(os.Stdout, "invalid strategy, %v", err)
		return
	}

	// Initialise db
	str, err := common.LoadDB(envConfig.DB)
	if err != nil {
		fmt.Fprintf(os.Stdout, "could not load db, %v", err)
		return
	}

	keys, err := common.LoadKeys(envConfig.Mnemonic)
	if err != nil {
		fmt.Fprintf(os.Stdout, "could not load keys, %v", err)
		return
	}
	// Calculate uid
	uid, err := strategy.Uid(strat)
	if err != nil {
		fmt.Fprintf(os.Stdout, "could not calculate uid, %v", err)
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
	serviceType := strings.Split(strat.StrategyType, "-")
	if len(serviceType) < 2 {
		fmt.Fprintf(os.Stdout, "invalid strat type")
		return
	}

	err = startService(serviceType[1], wg, s, strat, isIW)
	if err != nil {
		fmt.Fprintf(os.Stdout, "error starting service, %v", err)
		return
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
		fmt.Fprintf(os.Stdout, "failed to remove pid file, %v", err)
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

func startService(serviceType string, wg *sync.WaitGroup, s strategy.StrategyService, strat strategy.Strategy, isIW bool) error {
	switch strategy.StrategyType(serviceType) {
	case strategy.Filler:
		{
			service, err := strategy.StrategyToAutoFillStrategy(strat)
			if err != nil {
				return err
			}
			wg.Add(1)
			go s.RunAutoFillStrategy(service, isIW)
		}
	case strategy.Creator:
		{
			wg.Add(1)
			service, err := strategy.StrategyToAutoCreateStrategy(strat)
			if err != nil {
				return err
			}
			go s.RunAutoCreateStrategy(service, isIW)
		}
	}
	return nil
}
