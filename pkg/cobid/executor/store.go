package executor

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/catalogfi/cobi/pkg/swap"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/redis/go-redis/v9"
)

var (
	KeyBatchData = "batchData"
)

type BatchData struct {
	PrevOrders map[string]struct{} `json:"prev_orders"`
	RbfOptions btcswap.OptionRBF   `json:"rbf_options"`
}

func NewBatchData() BatchData {
	return BatchData{
		PrevOrders: map[string]struct{}{},
		RbfOptions: btcswap.OptionRBF{},
	}
}

func (bd *BatchData) HasAction(item btcswap.ActionItem) bool {
	key := fmt.Sprintf("%v_%v", item.Action, hex.EncodeToString(item.AtomicSwap.SecretHash))
	_, ok := bd.PrevOrders[key]
	return ok
}

func (bd *BatchData) AddExecuteAction(item btcswap.ActionItem) {
	key := fmt.Sprintf("%v_%v", item.Action, hex.EncodeToString(item.AtomicSwap.SecretHash))
	bd.PrevOrders[key] = struct{}{}
	return
}

type Store interface {

	// StoreAction keeps track of an action has been done on the swap of the given id.
	StoreAction(action swap.Action, swapID uint) error

	// CheckAction returns if an action has been done on the swap previously
	CheckAction(action swap.Action, swapID uint) (bool, error)

	// StoreBatchData stores the batch data into the storage
	StoreBatchData(bd BatchData) error

	// GetBatchData from the storage
	GetBatchData() (BatchData, error)
}

type redisStore struct {
	client *redis.Client
}

func NewRedisStore(redisURL string) (Store, error) {
	parsedURL, err := url.Parse(redisURL)
	if err != nil {
		return nil, err
	}
	redisPassword, _ := parsedURL.User.Password()
	client := redis.NewClient(&redis.Options{
		Addr:     parsedURL.Host,
		Password: redisPassword,
		DB:       0, // Use default DB.
	})
	return redisStore{client: client}, nil
}

func (rs redisStore) StoreAction(action swap.Action, swapID uint) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := actionKey(action, swapID)
	return rs.client.Set(ctx, key, true, 0).Err()
}

func (rs redisStore) CheckAction(action swap.Action, swapID uint) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := actionKey(action, swapID)
	ok, err := rs.client.Get(ctx, key).Bool()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	return ok, err
}

func (rs redisStore) StoreBatchData(bd BatchData) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := json.Marshal(bd)
	if err != nil {
		return err
	}
	return rs.client.Set(ctx, KeyBatchData, data, 0).Err()
}

func (rs redisStore) GetBatchData() (BatchData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := rs.client.Get(ctx, KeyBatchData).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return NewBatchData(), nil
		}
		return BatchData{}, err
	}
	var bd BatchData
	if err := json.Unmarshal(data, &bd); err != nil {
		return BatchData{}, err
	}
	return bd, nil
}

func (rs redisStore) ordersToString(orders map[uint]struct{}) string {
	if len(orders) == 0 {
		return ""
	}
	orderSlice := make([]string, 0, len(orders))
	for order := range orders {
		orderSlice = append(orderSlice, fmt.Sprintf("%v", order))
	}
	return strings.Join(orderSlice, ",")
}

func (rs redisStore) ordersFromString(orders string) map[uint]struct{} {
	if orders == "" {
		return map[uint]struct{}{}
	}
	orderSlice := strings.Split(orders, ",")

	orderMap := map[uint]struct{}{}
	for _, order := range orderSlice {
		orderID, err := strconv.ParseUint(order, 10, 0)
		if err != nil {
			panic(err)
		}
		orderMap[uint(orderID)] = struct{}{}
	}
	return orderMap
}

func actionKey(action swap.Action, swapID uint) string {
	return fmt.Sprintf("%v-%v", action, swapID)
}
