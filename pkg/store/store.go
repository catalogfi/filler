package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
)

var ErrTokenExpired = errors.New("token expired")

type Status uint

// dont change sequence of status fields might conflict retry feature
const (
	Unknown Status = iota
	Created
	Filled
	InitiatorInitiated
	InitiatorFailedToInitiate
	FollowerInitiated
	FollowerFailedToInitiate
	InitiatorRedeemed
	InitiatorFailedToRedeem
	FollowerRedeemed
	FollowerFailedToRedeem
	InitiatorRefunded
	InitiatorFailedToRefund
	FollowerRefunded
	FollowerFailedToRefund
)

type Event uint

const (
	UnknownEvent Event = iota
	Initiated
	Redeemed
	Refunded
)

type Order struct {
	gorm.Model

	OrderId    uint64 `gorm:"index:,unique,composite:account_order"`
	SecretHash string `gorm:"index:,unique,composite:account_order"`
	Secret     string
	Status     Status
	Error      string

	InitiateTxHash string
	RedeemTxHash   string
	RefundTxHash   string
}

type Token struct {
	gorm.Model

	Address string
	Token   string
}

type Store interface {
	// PutToken inserts the jwt token and associated address into the db.
	PutToken(addr common.Address, token string) error

	Token(addr common.Address) (string, error)

	PutSecret(secretHash string, secret *string, orderID uint64) error

	Status(secretHash string) (Status, error)

	Secret(secretHash string) (string, error)

	UpdateOrderStatus(secretHash string, status Status, err error) error

	UpdateTxHash(secretHash string, event Event, hash string) error

	OrderBySecretHash(secretHash string) (Order, error)

	OrderByID(id uint) (Order, error)
}

type store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) (Store, error) {
	if err := db.AutoMigrate(&Order{}, &Token{}); err != nil {
		return nil, err
	}

	// Set max connections
	sqlDb, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDb.SetMaxIdleConns(5)
	sqlDb.SetMaxOpenConns(5)
	sqlDb.SetConnMaxIdleTime(10 * time.Minute)
	return &store{db: db}, nil
}

func (store *store) PutToken(addr common.Address, token string) error {
	t := Token{
		Address: addr.Hex(),
		Token:   token,
	}
	return store.db.Create(&t).Error
}

func (store *store) Token(addr common.Address) (string, error) {
	var token Token
	if err := store.db.Where("address = ?", addr.Hex()).First(&token).Error; err != nil {
		return "", err
	}
	if time.Now().Sub(token.UpdatedAt) >= 12*time.Hour {
		return token.Token, ErrTokenExpired
	}
	return token.Token, nil
}

func (store *store) PutSecret(secretHash string, secret *string, orderID uint64) error {
	status := Created
	s := ""
	if secret == nil {
		status = Filled
	} else {
		s = *secret
	}

	order := Order{
		SecretHash: secretHash,
		OrderId:    orderID,
		Status:     status,
		Secret:     s,
	}
	return store.db.Create(&order).Error
}
func (store *store) Secret(secretHash string) (string, error) {
	var order Order
	if err := store.db.Where("secret_hash = ?", secretHash).First(&order).Error; err != nil {
		return "", err
	}
	return order.Secret, nil
}

func (store *store) UpdateOrderStatus(secretHash string, status Status, err error) error {
	if err != nil {
		return store.db.Table("orders").Where("secret_hash = ?", secretHash).
			Update("status", status).
			Update("error", err.Error()).
			Error
	}
	return store.db.Table("orders").Where("secret_hash = ?", secretHash).Update("status", status).Error
}

func (store *store) UpdateTxHash(secretHash string, event Event, hash string) error {
	tx := store.db.Table("orders").Where("secret_hash = ?", secretHash)
	switch event {
	case Initiated:
		return tx.Update("initiate_tx_hash", hash).Error
	case Redeemed:
		return tx.Update("redeem_tx_hash", hash).Error
	case Refunded:
		return tx.Update("refund_tx_hash", hash).Error
	default:
		return fmt.Errorf("unknown event")
	}
}

func (store *store) OrderBySecretHash(secretHash string) (Order, error) {
	var order Order
	err := store.db.Where("secret_hash = ?", secretHash).First(&order).Error
	return order, err
}

func (store *store) OrderByID(id uint) (Order, error) {
	var order Order
	err := store.db.Where("order_id = ?", id).First(&order).Error
	return order, err
}

func (store *store) Status(secretHash string) (Status, error) {
	var order Order
	err := store.db.Where("secret_hash = ?", secretHash).First(&order).Error
	return order.Status, err
}
