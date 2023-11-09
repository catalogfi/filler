package command

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/catalogfi/cobi/cobid/handlers"
)

type Command interface {
	Name() string
	Query(cfg handlers.CoreConfig, params json.RawMessage) (json.RawMessage, error)
}

type accountInfo struct{}

func GetAccountInfo() Command {
	return &accountInfo{}
}

func (a *accountInfo) Name() string {
	return "getAccountInfo"
}

func (a *accountInfo) Query(cfg handlers.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req handlers.RequestAccount
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

func (a *createOrder) Query(cfg handlers.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req handlers.RequestCreate
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	fmt.Println("payload : "+string(params), req)

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

func (a *fillOrder) Query(cfg handlers.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req handlers.RequestFill
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	fmt.Println("payload  : "+string(params), req)

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

func (a *depositFunds) Query(cfg handlers.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req handlers.RequestDeposit
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	fmt.Println("payload  : "+string(params), req)

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

func (a *transferFunds) Query(cfg handlers.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req handlers.RequestTransfer
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	fmt.Println("payload  : "+string(params), req)

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

func (a *listOrders) Query(cfg handlers.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req handlers.RequestListOrders
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	fmt.Println("payload  : "+string(params), req)

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

func (a *killService) Query(cfg handlers.CoreConfig, params json.RawMessage) (json.RawMessage, error) {
	var req handlers.KillSerivce
	if err := json.Unmarshal(params, &req); err != nil {
		fmt.Println("\n\n FAILED HERE \n\n")
		return nil, err
	}
	if req.ServiceType == "" {
		return nil, errors.New("Invalid Arguments Passed")
	}
	fmt.Println("payload  : "+string(params), req)

	err := handlers.Kill(req.ServiceType)
	if err != nil {
		return nil, err
	}

	return json.Marshal("Killed Sucessfull")
}
