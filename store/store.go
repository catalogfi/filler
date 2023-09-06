package store

import (
	"fmt"
	"sync"
	"time"

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

type Event uint

const (
	UnknownEvent Event = iota
	Initated
	Redeemed
	Refunded
)

type Order struct {
	gorm.Model

	Account    uint32 `gorm:"index:,unique,composite:account_order"`
	OrderId    uint64
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

	Account  uint32
	Selector uint32
	Token    string
}

type Store interface {
	UserStore(account uint32) UserStore
}

type UserStore interface {
	PutToken(selector uint32, token string) error
	Token(selector uint32) (string, error)

	PutSecret(secretHash, secret string, orderId uint64) error
	PutSecretHash(secretHash string, orderId uint64) error
	Secret(secretHash string) (string, error)
	PutStatus(secretHash string, status Status) error
	PutError(secretHash, err string, status Status) error
	CheckStatus(secretHash string) (bool, string)
	Status(secretHash string) Status
	GetOrder(id uint) (Order, error)
	Orders() ([]Order, error)
	CheckRetryStatus(secretHash string) (bool, error)
	PutTxHash(secretHash string, status Event, txHash string) error
}

type store struct {
	mu *sync.RWMutex
	db *gorm.DB
}

type userStore struct {
	mu      *sync.RWMutex
	db      *gorm.DB
	account uint32
}

func NewStore(dialector gorm.Dialector, opts ...gorm.Option) (Store, error) {
	db, err := gorm.Open(dialector, opts...)
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&Order{}, &Token{}); err != nil {
		return nil, err
	}
	return &store{mu: new(sync.RWMutex), db: db}, nil
}

func (s *store) UserStore(user uint32) UserStore {
	return &userStore{mu: s.mu, db: s.db, account: user}
}

func (s *userStore) PutToken(selector uint32, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tx := s.db.Create(&Token{
		Account:  s.account,
		Selector: selector,
		Token:    token,
	}); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *userStore) Token(selector uint32) (string, error) {
	var token Token
	if tx := s.db.Where("account = ? AND selector = ?", s.account, selector).First(&token); tx.Error != nil {
		return "", tx.Error
	}
	if time.Now().Unix()-token.UpdatedAt.Unix() > 12*3600 {
		return token.Token, fmt.Errorf("expired")
	}
	return token.Token, nil
}

func (s *userStore) PutSecretHash(secretHash string, orderId uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	order := Order{
		Account:    s.account,
		SecretHash: secretHash,
		OrderId:    orderId,
		Status:     Filled,
	}
	if tx := s.db.Create(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}
func (s *userStore) CheckStatus(secretHash string) (bool, string) {
	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", s.account, secretHash).First(&order); tx.Error != nil {
		return false, tx.Error.Error()
	}
	if order.Status >= InitiatorFailedToInitiate {
		return true, order.Error
	}
	return true, ""
}

func (s *userStore) CheckRetryStatus(secretHash string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", s.account, secretHash).First(&order); tx.Error != nil {
		return false, fmt.Errorf("Order not found in local storage")
	}
	if order.Status >= FollowerFailedToInitiate {
		return true, nil
	}

	return false, fmt.Errorf("Order still under Watch")

}
func (s *userStore) PutSecret(secretHash, secret string, orderId uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	order := Order{
		Account:    s.account,
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
func (s *userStore) PutError(secretHash, err string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", s.account, secretHash).First(&order); tx.Error != nil {
		return tx.Error
	}
	order.Error = err
	order.Status = status
	if tx := s.db.Save(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *userStore) Secret(secretHash string) (string, error) {
	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", s.account, secretHash).First(&order); tx.Error != nil {
		return "", tx.Error
	}
	return order.Secret, nil
}

func (s *userStore) PutStatus(secretHash string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", s.account, secretHash).First(&order); tx.Error != nil {
		return tx.Error
	}
	order.Status = status
	if tx := s.db.Save(&order); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *userStore) Status(secretHash string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", s.account, secretHash).First(&order); tx.Error != nil {
		return 0
	}
	return order.Status
}

func (s *userStore) GetOrder(id uint) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order Order
	if tx := s.db.Where("account = ? AND order_id = ?", s.account, id).First(&order); tx.Error != nil {
		return Order{}, tx.Error
	}
	return order, nil
}

func (s *userStore) Orders() ([]Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	orders := []Order{}
	if tx := s.db.Where("account = ?", s.account).Find(&orders); tx.Error != nil {
		return orders, tx.Error
	}
	return orders, nil
}
func (s *userStore) PutTxHash(secretHash string, event Event, txHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order Order
	if tx := s.db.Where("account = ? AND secret_hash = ?", s.account, secretHash).First(&order); tx.Error != nil {
		return tx.Error
	}

	switch event {
	case Initated:
		order.InitiateTxHash = txHash
	case Redeemed:
		order.RedeemTxHash = txHash
	case Refunded:
		order.RefundTxHash = txHash
	default:
		return fmt.Errorf("unknown event")
	}
	if tx := s.db.Save(&order); tx.Error != nil {
		return tx.Error
	}

	return nil
}
