package cobi

import (
	"fmt"

	"gorm.io/gorm"
)

type Status uint

// dont change sequence of status fields might conflict retry feature
const (
	Unknown Status = iota
	Created
	Filled
	InitiatorInitiated
	FollowerInitiated
	InitiatorRedeemed
	FollowerRedeemed
	InitiatorRefunded
	FollowerRefunded
	InitiatorFailedToInitiate
	FollowerFailedToInitiate
	InitiatorFailedToRedeem
	FollowerFailedToRedeem
	InitiatorFailedToRefund
	FollowerFailedToRefund
)

type Order struct {
	gorm.Model

	OrderId    uint64 `gorm:"unique; not null"`
	SecretHash string `gorm:"unique; not null"`
	Secret     string
	Status     Status
	Error      string
}

type Store interface {
	PutSecret(secretHash, secret string, orderId uint64) error
	PutSecretHash(secretHash string, orderId uint64) error
	Secret(secretHash string) (string, error)
	PutStatus(secretHash string, status Status) error
	PutError(secretHash, err string, status Status) error
	CheckStatus(secretHash string) (bool, string)
	Status(secretHash string) Status
	GetOrder(id uint) (Order, error)
}

type store struct {
	db *gorm.DB
}

func NewStore(dialector gorm.Dialector, opts ...gorm.Option) (Store, error) {
	db, err := gorm.Open(dialector, opts...)
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&Order{}); err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

func (s *store) PutSecretHash(secretHash string, orderId uint64) error {
	order := Order{
		SecretHash: secretHash,
		OrderId:    orderId,
		Status:     Filled,
	}
	if tx := s.db.Create(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}
func (s *store) CheckStatus(secretHash string) (bool, string) {
	var order Order
	if tx := s.db.Where("secret_hash = ?", secretHash).First(&order); tx.Error != nil {
		return false, fmt.Sprintf("Order not found in local storage")
	}
	if order.Status >= FollowerFailedToInitiate {
		return false, order.Error
	}

	return true, ""

}
func (s *store) PutSecret(secretHash, secret string, orderId uint64) error {
	order := Order{
		SecretHash: secretHash,
		OrderId:    orderId,
		Secret:     secret,
		Status:     Created,
	}
	if tx := s.db.Create(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}
func (s *store) PutError(secretHash, err string, status Status) error {
	var order Order
	if tx := s.db.Where("secret_hash = ?", secretHash).First(&order); tx.Error != nil {
		return tx.Error
	}
	order.Error = err
	order.Status = status
	if tx := s.db.Save(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *store) Secret(secretHash string) (string, error) {
	var order Order
	if tx := s.db.Where("secret_hash = ?", secretHash).First(&order); tx.Error != nil {
		return "", tx.Error
	}
	return order.Secret, nil
}

func (s *store) PutStatus(secretHash string, status Status) error {
	var order Order
	if tx := s.db.Where("secret_hash = ?", secretHash).First(&order); tx.Error != nil {
		return tx.Error
	}
	order.Status = status
	if tx := s.db.Save(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *store) Status(secretHash string) Status {
	var order Order
	if tx := s.db.Where("secret_hash = ?", secretHash).First(&order); tx.Error != nil {
		return 0
	}
	return order.Status
}

func (s *store) GetOrder(id uint) (Order, error) {
	var order Order
	if tx := s.db.Where("order_id = ?", id).First(&order); tx.Error != nil {
		return Order{}, tx.Error
	}
	return order, nil
}
