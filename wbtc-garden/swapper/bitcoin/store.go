package bitcoin

import (
	"gorm.io/gorm"
)

type IwStatus uint

const (
	Created IwStatus = iota
	RefundTxGenerated
	Redeemed
)

type IwState struct {
	gorm.Model

	Pubkey        string   `gorm:"primaryKey; not null"`
	Status        IwStatus `gorm:"not null"`
	Secret        string   `gorm:"unique; not null"`
	WalletAddress string   `gorm:"not null"`
}

type Store interface {
	DB() *gorm.DB
	PutSecret(pubkey, secret string, status IwStatus, iwaddress string) error
	GetSecret(walletAddr string) (string, error)
	DeleteSecret(secret string) error
	PutStatus(pubkey string, status IwStatus) error
	GetStatus(pubkey string) (IwStatus, error)
}

type store struct {
	db *gorm.DB
}

func NewStore(dialector gorm.Dialector, opts ...gorm.Option) (Store, error) {
	db, err := gorm.Open(dialector, opts...)
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&IwState{}); err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}
func (s *store) DB() *gorm.DB {
	return s.db
}
func (s *store) PutSecret(pubkey, secret string, status IwStatus, iwaddress string) error {
	wallet := IwState{
		Pubkey:        pubkey,
		Secret:        secret,
		Status:        status,
		WalletAddress: iwaddress,
	}
	if tx := s.db.Create(&wallet); tx.Error != nil {
		return tx.Error
	}
	return nil
}
func (s *store) DeleteSecret(secret string) error {
	wallet := IwState{}
	if tx := s.db.Unscoped().Where("secret = ?", secret).Delete(&wallet); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *store) GetSecret(walletAddr string) (string, error) {
	var wallet IwState
	// last stored secret will be the latest one to use
	if tx := s.db.Where("wallet_address = ?", walletAddr).Last(&wallet); tx.Error != nil {
		return "", tx.Error
	}
	return wallet.Secret, nil
}

func (s *store) PutStatus(pubkey string, status IwStatus) error {
	var wallet IwState
	if tx := s.db.Model(&wallet).Where("pubkey = ?", pubkey).Update("status", status); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *store) GetStatus(pubkey string) (IwStatus, error) {
	var wallet IwState
	if tx := s.db.Where("pubkey = ?", pubkey).First(&wallet); tx.Error != nil {
		return 0, tx.Error
	}
	return wallet.Status, nil
}
