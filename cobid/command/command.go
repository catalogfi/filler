package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/catalogfi/cobi/cobid/handlers"
	"github.com/catalogfi/cobi/cobid/types"
	"github.com/catalogfi/cobi/utils"
)

type Command interface {
	Name() string
	Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error)
}

type accountInfo struct{}

func GetAccountInfo() Command {
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

func CreateNewOrder() Command {
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

func FillOrder() Command {
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

	return json.Marshal(("Order filled sucessFull"))
}

type depositFunds struct{}

func DepositFunds() Command {
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

func TransferFunds() Command {
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

func ListOrders() Command {
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

func KillService() Command {
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

	return json.Marshal("Killed Sucessfull")
}

type startExecutor struct{}

func ExecutorService() Command {
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

	cmd := exec.Command(filepath.Join(utils.DefaultCobiDirectory(), "executor"), strconv.Itoa(int(req.Account)), strconv.FormatBool(req.IsInstantWallet))

	if err := cmd.Start(); err != nil {
		return json.Marshal("error starting process" + err.Error())
	}

	if cmd == nil || cmd.ProcessState != nil && cmd.ProcessState.Exited() || cmd.Process == nil {
		return json.Marshal("error starting process")
	}

	//<sucesss>
	//

	return json.Marshal("started Sucessfull")
}

type startStrategy struct{}

func StrategyService() Command {
	return &startStrategy{}
}

func (a *startStrategy) Name() string {
	return "startExecutor"
}

func (a *startStrategy) Query(cfg types.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req types.RequestStartStrategy
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}

	var service *handlers.Service
	err := service.Set(req.Service)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(filepath.Join(utils.DefaultCobiDirectory(), "strategy"), req.Service, strconv.FormatBool(req.IsInstantWallet))

	if err := cmd.Start(); err != nil {
		return json.Marshal("error starting process" + err.Error())
	}

	if cmd == nil || cmd.ProcessState != nil && cmd.ProcessState.Exited() || cmd.Process == nil {
		return json.Marshal("error starting process")
	}

	return json.Marshal("started sucessfull")
}

type status struct{}

func Status() Command {
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

	var service *handlers.Service
	err := service.Set(req.Service)
	if err != nil {
		return nil, err
	}

	isActive := handlers.Status(*service, req.Account)
	return json.Marshal(isActive)

}
