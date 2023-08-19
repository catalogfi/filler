package cobi

import (
	"fmt"
	"sync"

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

	Account    uint32 `gorm:"primaryKey"`
	OrderId    uint64
	SecretHash string `gorm:"primaryKey"`
	Secret     string
	Status     Status
	Error      string
}

type Store interface {
	PutSecret(account uint32, secretHash, secret string, orderId uint64) error
	PutSecretHash(account uint32, secretHash string, orderId uint64) error
	Secret(account uint32, secretHash string) (string, error)
	PutStatus(account uint32, secretHash string, status Status) error
	PutError(account uint32, secretHash, err string, status Status) error
	CheckStatus(account uint32, secretHash string) (bool, string)
	Status(account uint32, secretHash string) Status
	GetOrder(account uint32, id uint) (Order, error)
}

type store struct {
	mu *sync.RWMutex
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
	return &store{mu: new(sync.RWMutex), db: db}, nil
}

func (s *store) PutSecretHash(account uint32, secretHash string, orderId uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	order := Order{
		Account:    account,
		SecretHash: secretHash,
		OrderId:    orderId,
		Status:     Filled,
	}
	if tx := s.db.Create(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}
func (s *store) CheckStatus(account uint32, secretHash string) (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", account, secretHash).First(&order); tx.Error != nil {
		return false, fmt.Sprintf("Order not found in local storage")
	}
	if order.Status >= FollowerFailedToInitiate {
		return false, order.Error
	}

	return true, ""

}
func (s *store) PutSecret(account uint32, secretHash, secret string, orderId uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	order := Order{
		Account:    account,
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
func (s *store) PutError(account uint32, secretHash, err string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", account, secretHash).First(&order); tx.Error != nil {
		return tx.Error
	}
	order.Error = err
	order.Status = status
	if tx := s.db.Save(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *store) Secret(account uint32, secretHash string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", account, secretHash).First(&order); tx.Error != nil {
		return "", tx.Error
	}
	return order.Secret, nil
}

func (s *store) PutStatus(account uint32, secretHash string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", account, secretHash).First(&order); tx.Error != nil {
		return tx.Error
	}
	order.Status = status
	if tx := s.db.Save(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *store) Status(account uint32, secretHash string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", account, secretHash).First(&order); tx.Error != nil {
		return 0
	}
	return order.Status
}

func (s *store) GetOrder(account uint32, id uint) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order Order
	if tx := s.db.Where("account = ? AND order_id = ?", account, id).First(&order); tx.Error != nil {
		return Order{}, tx.Error
	}
	return order, nil
}
