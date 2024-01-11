package executor

import (
	"encoding/hex"
	"sync"

	"github.com/catalogfi/cobi/pkg/swap"
)

type Store interface {

	// RecordAction keeps track of an action has been done on the swap of the given id.
	RecordAction(action swap.Action, swapID uint) error

	// CheckAction returns if an action has been done on the swap previously
	CheckAction(action swap.Action, swapID uint) (bool, error)

	// PutSecret stores the secret.
	PutSecret(hash, secret []byte) error

	// Secret retrieves the secret by its hash.
	Secret(hash []byte) ([]byte, error)
}

func NewInMemStore() Store {
	return &InMemStore{
		secretMu: new(sync.Mutex),
		secrets:  map[string][]byte{},
		actionMu: new(sync.Mutex),
		actions:  map[swap.Action]map[uint]bool{},
	}
}

type InMemStore struct {
	secretMu *sync.Mutex
	secrets  map[string][]byte

	actionMu *sync.Mutex
	actions  map[swap.Action]map[uint]bool
}

func (store *InMemStore) RecordAction(action swap.Action, swapID uint) error {
	store.actionMu.Lock()
	defer store.actionMu.Unlock()

	actionMap, ok := store.actions[action]
	if !ok {
		actionMap = map[uint]bool{}
		store.actions[action] = actionMap
	}

	actionMap[swapID] = true
	return nil
}

func (store *InMemStore) CheckAction(action swap.Action, swapID uint) (bool, error) {
	store.actionMu.Lock()
	defer store.actionMu.Unlock()

	actionMap, ok := store.actions[action]
	if !ok {
		return false, nil
	}
	return actionMap[swapID], nil
}

func (store *InMemStore) PutSecret(hash, secret []byte) error {
	store.secretMu.Lock()
	defer store.secretMu.Unlock()

	hashStr := hex.EncodeToString(hash)
	store.secrets[hashStr] = secret
	return nil
}

func (store *InMemStore) Secret(hash []byte) ([]byte, error) {
	store.secretMu.Lock()
	defer store.secretMu.Unlock()

	hashStr := hex.EncodeToString(hash)
	return store.secrets[hashStr], nil
}
