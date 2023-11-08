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
	OrderId     uint   `json:"orderId" binding:"required"`
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
			return errors.New("string is empty or contains only white spaces")
		}
	}
	return nil
}
