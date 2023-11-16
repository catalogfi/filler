package types

import (
	"errors"
	"strings"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"go.uber.org/zap"
)

type RequestAccount struct {
	IsInstantWallet bool   `json:"isInstantWallet" binding:"required"`
	Asset           string `json:"asset" binding:"required"`
	Page            uint32 `json:"page" binding:"required"`
	PerPage         uint32 `json:"perPage" binding:"required"`
	UserAccount     uint32 `json:"userAccount" binding:"required"`
	IsLegacy        bool   `json:"isLegacy"`
}

type RequestCreate struct {
	UserAccount   uint32 `json:"userAccount" binding:"required"`
	OrderPair     string `json:"orderPair" binding:"required"`
	SendAmount    string `json:"sendAmount" binding:"required"`
	ReceiveAmount string `json:"receiveAmount" binding:"required"`
}

type RequestFill struct {
	UserAccount uint32 `json:"userAccount" binding:"required"`
	OrderId     uint64 `json:"orderId" binding:"required"`
}

type RequestDeposit struct {
	UserAccount uint32 `json:"userAccount" binding:"required"`
	Asset       string `json:"asset" binding:"required"`
	Amount      uint64 `json:"amount" binding:"required"`
}

type RequestTransfer struct {
	UserAccount uint32 `json:"userAccount" binding:"required"`
	Asset       string `json:"asset" binding:"required"`
	Amount      uint64 `json:"amount" binding:"required"`
	ToAddr      string `json:"toAddr" binding:"required"`
	UseIw       bool   `json:"useIw"`
	Force       bool   `json:"force"`
}

type RequestListOrders struct {
	Maker      string  `json:"maker"`
	OrderPair  string  `json:"orderPair"`
	SecretHash string  `json:"secretHash"`
	OrderBy    string  `json:"orderBy"`
	MinPrice   float64 `json:"minPrice"`
	MaxPrice   float64 `json:"maxPrice"`
	Page       uint32  `json:"page"`
	PerPage    uint32  `json:"perPage"`
}

type RequestStartStrategy struct {
	Service         string `json:"service"`
	IsInstantWallet bool   `json:"isInstantWallet"`
}
type RequestStartExecutor struct {
	Account         uint32 `json:"userAccount"`
	IsInstantWallet bool   `json:"isInstantWallet"`
}
type RequestStatus struct {
	Service string `json:"service"`
	Account uint32 `json:"userAccount"`
}

type AccountInfo struct {
	AccountNo     string `json:"accountNo"`
	Address       string `json:"address"`
	Balance       string `json:"balance"`
	UsableBalance string `json:"usableBalance"`
}

type CoreConfig struct {
	Storage   store.Store
	EnvConfig utils.Config
	Keys      *utils.Keys
	Logger    *zap.Logger
}

func CheckStrings(elements ...string) error {
	for _, elem := range elements {
		if strings.TrimSpace(elem) == "" {
			return errors.New("Invalid Arguments Passed")
		}
	}
	return nil
}

func CheckUint32s(elements ...uint32) error {
	for _, elem := range elements {
		if elem == 0 {
			return errors.New("Invalid Arguments Passed")
		}
	}
	return nil
}

func CheckUint64s(elements ...uint64) error {
	for _, elem := range elements {
		if elem == 0 {
			return errors.New("Invalid Arguments Passed")
		}
	}
	return nil
}
