package creator

import (
	"context"
	"encoding/hex"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
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

func (rs redisStore) PutSecret(hash, secret []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return rs.client.Set(ctx, hex.EncodeToString(hash), hex.EncodeToString(secret), 0).Err()
}

func (rs redisStore) Secret(hash []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := rs.client.Get(ctx, hex.EncodeToString(hash))
	if err := cmd.Err(); err != nil {
		return nil, err
	}
	return hex.DecodeString(cmd.String())
}
