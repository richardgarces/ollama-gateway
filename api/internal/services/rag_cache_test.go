package services

import (
	"strings"
	"testing"
	"time"
)

func TestResponseCacheKeyNormalizesPrompt(t *testing.T) {
	k1 := responseCacheKey("  Hola   Mundo  ")
	k2 := responseCacheKey("hola mundo")
	if k1 != k2 {
		t.Fatalf("expected normalized prompt hash keys to match")
	}
}

func TestResponseCacheExpiresEntries(t *testing.T) {
	cache := NewResponseCache(20*time.Millisecond, 10)
	cache.Set("k", "value")
	if got, ok := cache.Get("k"); !ok || got != "value" {
		t.Fatalf("expected cache hit before expiration")
	}
	time.Sleep(35 * time.Millisecond)
	if _, ok := cache.Get("k"); ok {
		t.Fatalf("expected cache miss after ttl expiration")
	}
}

func TestResponseCacheEvictsLRU(t *testing.T) {
	cache := NewResponseCache(time.Minute, 2)
	cache.Set("a", "1")
	cache.Set("b", "2")
	_, _ = cache.Get("a")
	cache.Set("c", "3")
	if _, ok := cache.Get("b"); ok {
		t.Fatalf("expected key b to be evicted as LRU")
	}
	if got, ok := cache.Get("a"); !ok || got != "1" {
		t.Fatalf("expected key a to remain in cache")
	}
	if got, ok := cache.Get("c"); !ok || got != "3" {
		t.Fatalf("expected key c to remain in cache")
	}
}

func TestStreamCachedResponseChunks(t *testing.T) {
	input := strings.Repeat("x", cachedStreamChunkSize+10)
	chunks := make([]string, 0)
	err := streamCachedResponse(input, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("streamCachedResponse() error = %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0] != strings.Repeat("x", cachedStreamChunkSize) {
		t.Fatalf("unexpected first chunk size")
	}
	if chunks[1] != strings.Repeat("x", 10) {
		t.Fatalf("unexpected second chunk size")
	}
}
