package cache

import (
	"errors"
	"testing"
	"time"
)

func TestMemoryCacheCRUDAndTTL(t *testing.T) {
	c := NewMemory()
	if _, err := c.Get("missing"); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("expected ErrCacheMiss for missing key")
	}

	if err := c.Set("k", []byte("v"), time.Second); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	v, err := c.Get("k")
	if err != nil || string(v) != "v" {
		t.Fatalf("unexpected get result: %q err=%v", string(v), err)
	}

	if err := c.Delete("k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := c.Get("k"); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("expected ErrCacheMiss after delete")
	}

	_ = c.Set("exp", []byte("1"), 5*time.Millisecond)
	time.Sleep(12 * time.Millisecond)
	if _, err := c.Get("exp"); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("expected ErrCacheMiss for expired key")
	}
}
