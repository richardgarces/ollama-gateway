package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedis(redisURL string) (*RedisCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis parse url: %w", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisCache{client: client}, nil
}

func (r *RedisCache) Get(key string) ([]byte, error) {
	val, err := r.client.Get(context.Background(), key).Bytes()
	if err == redis.Nil {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

func (r *RedisCache) Set(key string, val []byte, ttl time.Duration) error {
	return r.client.Set(context.Background(), key, val, ttl).Err()
}

func (r *RedisCache) Delete(key string) error {
	return r.client.Del(context.Background(), key).Err()
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}
