package executor

import (
	"context"
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

var ErrNotFound = fmt.Errorf("not found")

type Store interface {

	// StoreAction keeps track of an action has been done on the swap of the given id.
	StoreAction(action swap.Action, swapID uint) error

	// CheckAction returns if an action has been done on the swap previously
	CheckAction(action swap.Action, swapID uint) (bool, error)

	// // StorePreviousTx the related info of the previously submitted tx
	// StorePreviousTx(fee int, tx *wire.MsgTx, orders map[uint]struct{}) error
	//
	// // GetPreviousTx returns the info of the previously submitted tx
	// GetPreviousTx() (int, *wire.MsgTx, map[uint]struct{}, error)

	StoreRbfInfo(rbf *btcswap.OptionRBF, orders map[uint]struct{}) error

	GetRbfInfo() (*btcswap.OptionRBF, map[uint]struct{}, error)
}

var (
	KeyOrders = "orders"
	KeyRBF    = "rbf"
)

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

	key := rs.actionKey(action, swapID)
	return rs.client.Set(ctx, key, true, 0).Err()
}

func (rs redisStore) CheckAction(action swap.Action, swapID uint) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := rs.actionKey(action, swapID)
	ok, err := rs.client.Get(ctx, key).Bool()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	return ok, err
}

func (rs redisStore) StoreRbfInfo(rbf *btcswap.OptionRBF, orders map[uint]struct{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var data []byte
	var err error
	if rbf != nil {
		data, err = json.Marshal(rbf)
		if err != nil {
			return err
		}
	}

	if err := rs.client.Set(ctx, KeyRBF, data, 0).Err(); err != nil {
		return err
	}

	val := rs.ordersToString(orders)
	return rs.client.Set(ctx, KeyOrders, val, 0).Err()
}

func (rs redisStore) GetRbfInfo() (*btcswap.OptionRBF, map[uint]struct{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := rs.client.Get(ctx, KeyRBF).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	var rbf btcswap.OptionRBF
	if len(data) != 0 {
		if err := json.Unmarshal(data, &rbf); err != nil {
			return nil, nil, err
		}
	}

	orderStr, err := rs.client.Get(ctx, KeyOrders).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	orders := rs.ordersFromString(orderStr)
	return &rbf, orders, nil
}

// func (rs redisStore) StorePreviousTx(fee int, tx *wire.MsgTx, orders map[uint]struct{}) error {
// 	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 	defer cancel()
//
// 	if err := rs.client.Set(ctx, KeyFees, fee, 0).Err(); err != nil {
// 		return err
// 	}
//
// 	buffer := bytes.NewBuffer([]byte{})
// 	if tx != nil {
// 		if err := tx.Serialize(buffer); err != nil {
// 			return err
// 		}
// 	}
// 	if err := rs.client.Set(ctx, KeyTx, buffer.Bytes(), 0).Err(); err != nil {
// 		return err
// 	}
//
// 	val := rs.ordersToString(orders)
// 	return rs.client.Set(ctx, KeyOrders, val, 0).Err()
// }
//
// func (r redisStore) GetPreviousTx() (int, *wire.MsgTx, map[uint]struct{}, error) {
// 	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 	defer cancel()
//
// 	fee, err := r.client.Get(ctx, KeyFees).Int()
// 	if err != nil {
// 		if errors.Is(err, redis.Nil) {
// 			return 0, nil, nil, ErrNotFound
// 		}
// 		return 0, nil, nil, err
// 	}
// 	txBytes, err := r.client.Get(ctx, KeyTx).Bytes()
// 	if err != nil {
// 		if errors.Is(err, redis.Nil) {
// 			return 0, nil, nil, ErrNotFound
// 		}
// 		return 0, nil, nil, err
// 	}
// 	var tx *wire.MsgTx
// 	if len(txBytes) != 0 {
// 		decodedTx, err := btcutil.NewTxFromBytes(txBytes)
// 		if err != nil {
// 			return 0, nil, nil, err
// 		}
// 		tx = decodedTx.MsgTx()
// 	}
//
// 	orderStr, err := r.client.Get(ctx, KeyOrders).Result()
// 	if err != nil {
// 		if errors.Is(err, redis.Nil) {
// 			return 0, nil, nil, ErrNotFound
// 		}
// 		return 0, nil, nil, err
// 	}
// 	orders := r.ordersFromString(orderStr)
// 	return fee, tx, orders, nil
// }

func (rs redisStore) actionKey(action swap.Action, swapID uint) string {
	return fmt.Sprintf("%v-%v", action, swapID)
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
