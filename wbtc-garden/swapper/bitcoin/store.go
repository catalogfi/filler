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

	Pubkey string   `gorm:"primaryKey; not null"`
	Status IwStatus `gorm:"not null"`
	Secret string   `gorm:"unique; not null"`
	Code   uint32   `gorm:"primaryKey; not null"`
}

type Store interface {
	PutSecret(pubkey, secret string, status IwStatus, code uint32) error
	Secret(pubkey string, code uint32) (string, error)
	PutStatus(pubkey string, code uint32, status IwStatus) error
	Status(pubkey string, code uint32) (IwStatus, error)
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
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_iw_state_pubkey_code ON iw_states (pubkey, code)")
	return &store{db: db}, nil
}

func (s *store) PutSecret(pubkey, secret string, status IwStatus, code uint32) error {
	wallet := IwState{
		Pubkey: pubkey,
		Secret: secret,
		Status: status,
		Code:   uint32(code),
	}
	if tx := s.db.Create(&wallet); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *store) Secret(pubkey string, code uint32) (string, error) {
	var wallet IwState
	if tx := s.db.Where("code = ? and pubkey = ?", code, pubkey).First(&wallet); tx.Error != nil {
		return "", tx.Error
	}
	return wallet.Secret, nil
}

func (s *store) PutStatus(pubkey string, code uint32, status IwStatus) error {
	var wallet IwState
	if tx := s.db.Model(&wallet).Where("code = ? and pubkey = ?", code, pubkey).Update("status", status); tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (s *store) Status(pubkey string, code uint32) (IwStatus, error) {
	var wallet IwState
	if tx := s.db.Where("code = ? and pubkey = ?", code, pubkey).First(&wallet); tx.Error != nil {
		return 0, tx.Error
	}
	return wallet.Status, nil
}
