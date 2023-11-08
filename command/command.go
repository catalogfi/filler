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

	return json.Marshal(fmt.Sprintf("Order filled sucessFull"))
}
