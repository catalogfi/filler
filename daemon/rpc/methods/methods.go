package methods

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/catalogfi/cobi/daemon/executor"
	"github.com/catalogfi/cobi/daemon/rpc/handlers"
	"github.com/catalogfi/cobi/daemon/strategy"
	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/pkg/process"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/orderbook/model"
)

type Method interface {
	Name() string
	Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error)
}

type accountInfo struct{}

func GetAccountInfo() Method {
	return &accountInfo{}
}

func (a *accountInfo) Name() string {
	return "getAccountInfo"
}

func (a *accountInfo) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestAccount
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	accounts, err := handlers.GetAccounts(*cfg, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(accounts)
}

type CreateOrder struct{}

func CreateNewOrder() Method {
	return &CreateOrder{}
}

func (a *CreateOrder) Name() string {
	return "createOrder"
}

func (a *CreateOrder) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestCreate
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	id, err := handlers.Create(*cfg, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(id)
}

type fillOrder struct{}

func FillOrder() Method {
	return &fillOrder{}
}

func (a *fillOrder) Name() string {
	return "fillOrder"
}

func (a *fillOrder) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestFill
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	err := handlers.FillOrder(*cfg, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(("Order filled successful"))
}

type depositFunds struct{}

func DepositFunds() Method {
	return &depositFunds{}
}

func (a *depositFunds) Name() string {
	return "depositFunds"
}

func (a *depositFunds) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestDeposit
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	txhash, err := handlers.Deposit(*cfg, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(fmt.Sprintf("txHash : %s", txhash))
}

type transferFunds struct{}

func TransferFunds() Method {
	return &transferFunds{}
}

func (a *transferFunds) Name() string {
	return "transferFunds"
}

func (a *transferFunds) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestTransfer
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	txhash, err := handlers.Transfer(*cfg, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(fmt.Sprintf("txHash : %s", txhash))
}

type listOrders struct{}

func ListOrders() Method {
	return &listOrders{}
}

func (a *listOrders) Name() string {
	return "listOrders"
}

func (a *listOrders) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestListOrders
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	Orders, err := handlers.List(*cfg, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(Orders)
}

type status struct{}

func Status() Method {
	return &status{}
}

func (a *status) Name() string {
	return "status"
}

func (a *status) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestStatus
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	// check strategy status

	return json.Marshal(true)

}

type setConfig struct{}

func SetConfig() Method {
	return &setConfig{}
}

func (a *setConfig) Name() string {
	return "setConfig"
}

func (a *setConfig) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.SetConfig
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	config, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return nil, err
	}

	if req.Mnemonic != "" {
		config.Mnemonic = req.Mnemonic
		cfg.EnvConfig.Mnemonic = req.Mnemonic
	}
	if req.OrderBook != "" {
		config.OrderBook = req.OrderBook
		cfg.EnvConfig.OrderBook = req.OrderBook

	}
	if req.DB != "" {
		config.DB = req.DB
		cfg.EnvConfig.DB = req.DB
	}
	if req.Sentry != "" {
		config.Sentry = req.Sentry
		cfg.EnvConfig.Sentry = req.Sentry
	}
	if req.RpcUserName != "" {
		config.RpcUserName = req.RpcUserName
		cfg.EnvConfig.RpcUserName = req.RpcUserName
	}
	if req.RpcPassword != "" {
		config.RpcPassword = req.RpcPassword
		cfg.EnvConfig.RpcPassword = req.RpcPassword
	}

	// update authentication
	rpcLogin := config.RpcUserName + ":" + config.RpcPassword
	rpcAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(rpcLogin))
	cfg.Authsha = sha256.Sum256([]byte(rpcAuth))

	if req.NoTLS != "" {
		switch req.NoTLS {
		case "true":
			config.NoTLS = true
			cfg.EnvConfig.NoTLS = true
		case "false":
			config.NoTLS = false
			cfg.EnvConfig.NoTLS = false
		default:
			return nil, errors.New("invalid arguments passed")
		}
	}
	if req.RPCServer != "" {
		config.RPCServer = req.RPCServer
		cfg.EnvConfig.RPCServer = req.RPCServer
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(utils.DefaultConfigPath(), data, 0755); err != nil {
		return nil, err
	}
	return json.Marshal("success")
}

type retry struct{}

func Retry() Method {
	return &retry{}
}

func (a *retry) Name() string {
	return "retryOrder"
}

func (a *retry) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestRetry
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	if err := handlers.Retry(*cfg, req); err != nil {
		return nil, err
	}

	return json.Marshal("successfully retried")
}

type setNetwork struct{}

func SetNetwork() Method {
	return &setNetwork{}
}

func (a *setNetwork) Name() string {
	return "setNetwork"
}

func (a *setNetwork) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var networkConfig model.Network
	if err := json.Unmarshal(params, &networkConfig); err != nil {
		return nil, err
	}
	config, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return nil, err
	}
	config.Network = networkConfig
	bytes, err := json.MarshalIndent(config, "", " ")
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(utils.DefaultConfigPath(), bytes, 0644)
	if err != nil {
		return nil, err
	}
	return json.Marshal("success")
}

type getNetworks struct{}

func GetNetworks() Method {
	return &getNetworks{}
}

func (a *getNetworks) Name() string {
	return "getNetworks"
}

func (a *getNetworks) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	config, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return nil, err
	}
	return json.Marshal(config.Network)
}

type setStrategy struct{}

func SetStrategy() Method {
	return &setStrategy{}
}

func (a *setStrategy) Name() string {
	return "setStrategy"
}

func (a *setStrategy) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var strategyConfig json.RawMessage
	if err := json.Unmarshal(params, &strategyConfig); err != nil {
		return nil, err
	}

	stratBytes, err := json.MarshalIndent(strategyConfig, "", " ")
	if err != nil {
		return nil, err
	}

	strategies := []strategy.Strategy{}
	if err := json.Unmarshal(stratBytes, &strategies); err != nil {
		return nil, err
	}

	config, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return nil, err
	}

	oldStratBytes, err := json.MarshalIndent(config.Strategies, "", " ")
	if err != nil {
		return nil, err
	}

	oldStrategies := []strategy.Strategy{}
	if err := json.Unmarshal(oldStratBytes, &oldStrategies); err != nil {
		return nil, err
	}

	startAccounts := make(map[uint32]process.ProcessManager)
	stopAccounts := make(map[uint32]process.ProcessManager)
	newStrats := make(map[string]process.ProcessManager)
	commonStrats := make(map[string]bool)
	quitStrats := make(map[string]process.ProcessManager)
	isIw := make(map[uint32]bool)

	for _, s := range strategies {
		i, ok := isIw[s.Account]
		if ok && i != s.UseIw {
			return json.Marshal("uncertain strategy, strategies of same account can have only one value for UseIw")
		}
		execUid, _ := executor.Uid(s.UseIw, s.Account)
		execProcess := process.NewProcessManager(execUid)
		if _, ok := startAccounts[s.Account]; !ok {
			if !execProcess.IsActive() {
				startAccounts[s.Account] = execProcess
				isIw[s.Account] = s.UseIw
			}
		}

		execUid, _ = executor.Uid(!s.UseIw, s.Account)
		execProcess = process.NewProcessManager(execUid)
		if _, ok := stopAccounts[s.Account]; !ok {
			if execProcess.IsActive() {
				stopAccounts[s.Account] = execProcess
			}
		}

		stratUid, _ := strategy.Uid(s)
		stratProcess := process.NewProcessManager(stratUid)

		if !stratProcess.IsActive() {
			if _, ok := newStrats[stratUid]; !ok {
				newStrats[stratUid] = stratProcess
			}
		} else {
			commonStrats[stratUid] = true
		}
	}
	for _, s := range oldStrategies {
		stratUid, _ := strategy.Uid(s)
		stratProcess := process.NewProcessManager(stratUid)
		if stratProcess.IsActive() && !commonStrats[stratUid] {
			quitStrats[stratUid] = stratProcess
		}
	}
	config.Strategies = strategyConfig
	bytes, err := json.MarshalIndent(config, "", " ")
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(utils.DefaultConfigPath(), bytes, 0644)
	if err != nil {
		return nil, err
	}

	for _, process := range stopAccounts {
		err := a.stopExecutor(process)
		if err != nil {
			return json.Marshal(fmt.Sprintf("failed to stop executor, err : %v", err))
		}
	}

	for id, process := range startAccounts {
		err := a.startExecutor(process, id, isIw[id])
		if err != nil {
			return json.Marshal(fmt.Sprintf("failed to start executor, err : %v", err))
		}
	}
	for _, process := range quitStrats {
		err := a.stopStrategy(process)
		if err != nil {
			return json.Marshal(fmt.Sprintf("failed to stop strategy, err : %v", err))
		}
	}
	for _, process := range newStrats {
		err := a.startStrategy(process, false)
		if err != nil {
			return json.Marshal(fmt.Sprintf("failed to start strategy, err : %v", err))
		}
	}

	return json.Marshal("successfully started strategies")
}

func (a *setStrategy) startExecutor(execProcess process.ProcessManager, account uint32, isIw bool) error {
	n, msgBytes, err := execProcess.Start(
		filepath.Join(utils.DefaultCobiBin(), "executor"),
		[]string{strconv.FormatUint(uint64(account), 10), strconv.FormatBool(isIw)})
	if err != nil {
		return err
	}

	msg := string(msgBytes[:n])
	if msg == process.DefaultSuccessfulMsg {
		return nil
	}
	return fmt.Errorf("%s", msg)
}
func (a *setStrategy) stopExecutor(execProcess process.ProcessManager) error {
	return execProcess.Stop()
}

func (a *setStrategy) startStrategy(stratProcess process.ProcessManager, isIw bool) error {
	n, msgBytes, err := stratProcess.Start(
		filepath.Join(utils.DefaultCobiBin(), "strategy"),
		[]string{stratProcess.GetUid()})

	if err != nil {
		return err
	}

	msg := string(msgBytes[:n])
	if msg == process.DefaultSuccessfulMsg {
		return nil
	}
	return fmt.Errorf("%s", msg)
}
func (a *setStrategy) stopStrategy(stratProcess process.ProcessManager) error {
	return stratProcess.Stop()
}

type getStrategy struct{}

func GetStrategy() Method {
	return &getStrategy{}
}

func (a *getStrategy) Name() string {
	return "getStrategy"
}

func (a *getStrategy) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	config, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return nil, err
	}
	return config.Strategies, nil
}

type getConfig struct{}

func GetConfig() Method {
	return &getConfig{}
}

func (a *getConfig) Name() string {
	return "getConfig"
}

func (a *getConfig) Query(cfg *types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	config, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return nil, err
	}

	var resp struct {
		OrderBook string `json:"orderBook"`
		DB        string `json:"db"`
		Sentry    string `json:"sentry"`
		RpcServer string `json:"rpcServer"`
		NoTLS     bool   `json:"noTLS"`
	}

	resp.OrderBook = config.OrderBook
	resp.DB = config.DB
	resp.Sentry = config.Sentry
	resp.RpcServer = config.RPCServer
	resp.NoTLS = config.NoTLS

	return json.Marshal(resp)
}
