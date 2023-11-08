package handlers

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

type CoreConfig struct {
	Storage   store.Store
	EnvConfig utils.Config
	Keys      *utils.Keys
	Logger    *zap.Logger
}

func checkStrings(elements ...string) error {
	for _, elem := range elements {
		if strings.TrimSpace(elem) == "" {
			return errors.New("Invalid Arguments Passed")
		}
	}
	return nil
}

func checkUint32s(elements ...uint32) error {
	for _, elem := range elements {
		if elem == 0 {
			return errors.New("Invalid Arguments Passed")
		}
	}
	return nil
}

func checkUint64s(elements ...uint64) error {
	for _, elem := range elements {
		if elem == 0 {
			return errors.New("Invalid Arguments Passed")
		}
	}
	return nil
}
