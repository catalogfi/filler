package common

import (
	"time"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/tyler-smith/go-bip39"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func LoadDB(dbDialector string) (store.Store, error) {
	var str store.Store
	var err error
	if dbDialector != "" {
		str, err = store.NewStore(sqlite.Open(dbDialector), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			return nil, err
		}
	} else {
		str, err = store.NewStore(sqlite.Open(utils.DefaultStorePath()), &gorm.Config{
			NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			return nil, err
		}
	}
	return str, nil
}

func LoadKeys(mnemonic string) (utils.Keys, error) {
	entropy, err := bip39.EntropyFromMnemonic(mnemonic)
	if err != nil {
		return utils.Keys{}, err
	}

	return utils.NewKeys(entropy), nil
}
