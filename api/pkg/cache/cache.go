package cache

import (
	"errors"
	"time"
)

var ErrCacheMiss = errors.New("cache miss")

type Cache interface {
	Get(key string) ([]byte, error)
	Set(key string, val []byte, ttl time.Duration) error
	Delete(key string) error
}
