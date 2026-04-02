package services

import (
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/domain"
)

type cacheItem struct {
	key       string
	value     []float64
	timestamp time.Time
}

// EmbeddingCache is an LRU cache with TTL for embeddings.
type EmbeddingCache struct {
	mu         sync.RWMutex
	items      map[string]*list.Element
	order      *list.List // front is most recent
	ttl        time.Duration
	maxEntries int
}

func NewEmbeddingCache(ttl time.Duration, maxEntries int) *EmbeddingCache {
	return &EmbeddingCache{
		items:      make(map[string]*list.Element),
		order:      list.New(),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

func (c *EmbeddingCache) Get(key string) ([]float64, bool) {
	c.mu.RLock()
	el, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	item := el.Value.(*cacheItem)
	if time.Since(item.timestamp) > c.ttl {
		c.mu.Lock()
		c.order.Remove(el)
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}

	// update recency
	c.mu.Lock()
	c.order.MoveToFront(el)
	c.mu.Unlock()

	return append([]float64(nil), item.value...), true
}

func (c *EmbeddingCache) Set(key string, value []float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		el.Value.(*cacheItem).value = append([]float64(nil), value...)
		el.Value.(*cacheItem).timestamp = time.Now()
		c.order.MoveToFront(el)
		return
	}

	item := &cacheItem{key: key, value: append([]float64(nil), value...), timestamp: time.Now()}
	el := c.order.PushFront(item)
	c.items[key] = el

	if c.maxEntries > 0 && c.order.Len() > c.maxEntries {
		back := c.order.Back()
		if back != nil {
			it := back.Value.(*cacheItem)
			delete(c.items, it.key)
			c.order.Remove(back)
		}
	}
}

type OllamaService struct {
	baseURL string
	client  *http.Client
	cache   *EmbeddingCache
}

func NewOllamaService(cfg *config.Config) *OllamaService {
	ttl := time.Duration(cfg.EmbeddingCacheTTLSeconds) * time.Second
	max := cfg.EmbeddingCacheMaxEntries
	return &OllamaService{
		baseURL: cfg.OllamaURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		cache: NewEmbeddingCache(ttl, max),
	}
}

func (s *OllamaService) Generate(model, prompt string) (string, error) {
	reqBody := domain.OllamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Post(s.baseURL+"/api/generate", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result domain.OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama decode error: %w", err)
	}

	return result.Response, nil
}

func (s *OllamaService) StreamGenerate(model, prompt string, onChunk func(string) error) error {
	reqBody := domain.OllamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: true,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	resp, err := s.client.Post(s.baseURL+"/api/generate", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("ollama streaming request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		var chunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
		}
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("ollama streaming decode error: %w", err)
		}
		if chunk.Response != "" {
			if err := onChunk(chunk.Response); err != nil {
				return err
			}
		}
		if chunk.Done {
			return nil
		}
	}
}

// (no local buffer helpers; using bytes.NewBuffer and standard io)

func (s *OllamaService) GetEmbedding(model, text string) ([]float64, error) {
	cacheKey := model + ":" + text
	if v, ok := s.cache.Get(cacheKey); ok {
		return v, nil
	}

	reqBody := map[string]interface{}{
		"model":  model,
		"prompt": text,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Post(s.baseURL+"/api/embeddings", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("ollama embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embedding decode error: %w", err)
	}

	s.cache.Set(cacheKey, result.Embedding)

	return result.Embedding, nil
}
