package methods

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/catalogfi/cobi/daemon/rpc/handlers"
	"github.com/catalogfi/cobi/daemon/strategy"
	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/utils"
	"github.com/catalogfi/wbtc-garden/model"
)

type Method interface {
	Name() string
	Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error)
}

type accountInfo struct{}

func GetAccountInfo() Method {
	return &accountInfo{}
}

func (a *accountInfo) Name() string {
	return "getAccountInfo"
}

func (a *accountInfo) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestAccount
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	accounts, err := handlers.GetAccounts(cfg, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(accounts)
}

type createOrder struct{}

func CreateNewOrder() Method {
	return &createOrder{}
}

func (a *createOrder) Name() string {
	return "createNewOrder"
}

func (a *createOrder) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestCreate
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	id, err := handlers.Create(cfg, req)
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

func (a *fillOrder) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestFill
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	err := handlers.FillOrder(cfg, req)
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

func (a *depositFunds) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestDeposit
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	txhash, err := handlers.Deposit(cfg, req)
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

func (a *transferFunds) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestTransfer
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	txhash, err := handlers.Transfer(cfg, req)
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

func (a *listOrders) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestListOrders
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	Orders, err := handlers.List(cfg, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(Orders)
}

type killService struct{}

func KillService() Method {
	return &killService{}
}

func (a *killService) Name() string {
	return "killService"
}

func (a *killService) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req handlers.KillSerivce
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if req.ServiceType == "" {
		return nil, errors.New("invalid arguments passed")
	}

	err := handlers.Kill(req)
	if err != nil {
		return nil, err
	}

	return json.Marshal("Killed successfull")
}

type startExecutor struct{}

func ExecutorService() Method {
	return &startExecutor{}
}

func (a *startExecutor) Name() string {
	return "startExecutor"
}

func (a *startExecutor) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestStartExecutor
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	cmd := exec.Command(filepath.Join(utils.DefaultCobiBin(), "executor"), strconv.Itoa(int(req.Account)), strconv.FormatBool(req.IsInstantWallet))

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return json.Marshal(fmt.Sprintf("error creating stdout pipe, err:%v", err))
	}

	if err := cmd.Start(); err != nil {
		return json.Marshal(fmt.Sprintf("error starting process, err:%v", err))
	}

	if cmd == nil || cmd.ProcessState != nil && cmd.ProcessState.Exited() || cmd.Process == nil {
		return json.Marshal("error starting process")
	}

	buf := make([]byte, 1024)
	n, err := stdoutPipe.Read(buf)
	if err != nil && err != io.EOF {
		return json.Marshal("Error reading from pipe")
	}

	receivedData := string(buf[:n])
	if receivedData != "successful" {
		return json.Marshal(receivedData)
	}

	return json.Marshal("started successfully")
}

type startStrategy struct{}

func StrategyService() Method {
	return &startStrategy{}
}

func (a *startStrategy) Name() string {
	return "startStrategy"
}

func (a *startStrategy) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestStartStrategy
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	var service handlers.Service
	err := service.Set(req.Service)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(filepath.Join(utils.DefaultCobiBin(), "strategy"), req.Service, strconv.FormatBool(req.IsInstantWallet))

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return json.Marshal(fmt.Sprintf("error creating stdout pipe, err:%v", err))
	}

	if err := cmd.Start(); err != nil {
		return json.Marshal(fmt.Sprintf("error starting process, err:%v", err.Error()))
	}

	if cmd == nil || cmd.ProcessState != nil && cmd.ProcessState.Exited() || cmd.Process == nil {
		return json.Marshal("error starting process")
	}

	buf := make([]byte, 1024)
	n, err := stdoutPipe.Read(buf)
	if err != nil && err != io.EOF {
		return json.Marshal("error reading from pipe")
	}

	receivedData := string(buf[:n])
	if receivedData != "successful" {
		return json.Marshal(receivedData)
	}

	return json.Marshal("started successfully")
}

type status struct{}

func Status() Method {
	return &status{}
}

func (a *status) Name() string {
	return "status"
}

func (a *status) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestStatus
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	var service handlers.Service
	err := service.Set(req.Service)
	if err != nil {
		return nil, err
	}

	isActive := handlers.Status(service, req.Account)
	return json.Marshal(isActive)

}

type setConfig struct{}

func SetConfig() Method {
	return &setConfig{}
}

func (a *setConfig) Name() string {
	return "setConfig"
}

func (a *setConfig) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
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
	}
	if req.OrderBook != "" {
		config.OrderBook = req.OrderBook
	}
	if req.DB != "" {
		config.DB = req.DB
	}
	if req.Sentry != "" {
		config.Sentry = req.Sentry
	}
	if req.RpcUserName != "" {
		config.RpcUserName = req.RpcUserName
	}
	if req.RpcPassword != "" {
		config.RpcPassword = req.RpcPassword
	}
	if req.NoTLS != "" {
		switch req.NoTLS {
		case "true":
			config.NoTLS = true
		case "false":
			config.NoTLS = false
		default:
			return nil, errors.New("invalid arguments passed")
		}
	}
	if req.RPCServer != "" {
		config.RPCServer = req.RPCServer
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

func (a *retry) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestRetry
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	if err := handlers.Retry(cfg, req); err != nil {
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

func (a *setNetwork) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
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

func (a *getNetworks) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
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

func (a *setStrategy) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var strategyConfig json.RawMessage
	if err := json.Unmarshal(params, &strategyConfig); err != nil {
		return nil, err
	}

	strategies := []strategy.Strategy{}
	if err := json.Unmarshal(strategyConfig, &strategies); err != nil {
		return nil, err
	}

	config, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return nil, err
	}

	{
		//TODO: compute hashes as ID's
		// check for duplicates
		// check for updated strategies
		// check for new strategies
		// check for removed strategies
		// start requireed executors
		// start strategies
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
	return json.Marshal("success")
}

type getStrategy struct{}

func GetStrategy() Method {
	return &getStrategy{}
}

func (a *getStrategy) Name() string {
	return "getStrategy"
}

func (a *getStrategy) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
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

func (a *getConfig) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	config, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return nil, err
	}
	var requestConfig types.SetConfig
	if err := json.Unmarshal(params, &requestConfig); err != nil {
		return nil, err
	}

	requestConfig.Mnemonic = ""
	requestConfig.RpcUserName = ""
	requestConfig.RpcPassword = ""
	return json.Marshal(config)
}
