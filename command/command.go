package command

import (
	"encoding/json"
	"fmt"

	"github.com/catalogfi/cobi/handlers"
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
	fmt.Println("payload  : "+string(params), req)

	id, err := handlers.Create(cfg, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(fmt.Sprintf("ordercreatd %d", id))
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
	return &depositFunds{}
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
