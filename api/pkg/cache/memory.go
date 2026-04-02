package cache

import (
	"sync"
	"time"
)

type memoryItem struct {
	value     []byte
	expiresAt time.Time
}

type MemoryCache struct {
	items sync.Map
}

func NewMemory() *MemoryCache {
	return &MemoryCache{}
}

func (m *MemoryCache) Get(key string) ([]byte, error) {
	raw, ok := m.items.Load(key)
	if !ok {
		return nil, ErrCacheMiss
	}
	item, ok := raw.(memoryItem)
	if !ok {
		m.items.Delete(key)
		return nil, ErrCacheMiss
	}
	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		m.items.Delete(key)
		return nil, ErrCacheMiss
	}
	out := make([]byte, len(item.value))
	copy(out, item.value)
	return out, nil
}

func (m *MemoryCache) Set(key string, val []byte, ttl time.Duration) error {
	item := memoryItem{}
	item.value = make([]byte, len(val))
	copy(item.value, val)
	if ttl > 0 {
		item.expiresAt = time.Now().Add(ttl)
	}
	m.items.Store(key, item)
	return nil
}

func (m *MemoryCache) Delete(key string) error {
	m.items.Delete(key)
	return nil
}
