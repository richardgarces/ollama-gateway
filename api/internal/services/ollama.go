package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/cache"
	"ollama-gateway/pkg/httpclient"
)

type OllamaService struct {
	baseURL      string
	client       *http.Client
	cache        cache.Cache
	cacheTTL     time.Duration
	maxCacheSize int
	logger       *slog.Logger
}

var _ domain.OllamaClient = (*OllamaService)(nil)

func NewOllamaService(cfg *config.Config, logger *slog.Logger, embeddingCache cache.Cache) *OllamaService {
	if logger == nil {
		logger = slog.Default()
	}
	if embeddingCache == nil {
		embeddingCache = cache.NewMemory()
	}
	ttl := time.Duration(cfg.EmbeddingCacheTTLSeconds) * time.Second
	max := cfg.EmbeddingCacheMaxEntries
	return &OllamaService{
		baseURL: cfg.OllamaURL,
		client: httpclient.NewResilientClient(httpclient.Options{
			Timeout:    time.Duration(cfg.HTTPTimeoutSeconds) * time.Second,
			MaxRetries: cfg.HTTPMaxRetries,
		}),
		cache:        embeddingCache,
		cacheTTL:     ttl,
		maxCacheSize: max,
		logger:       logger,
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
	if raw, err := s.cache.Get(cacheKey); err == nil {
		var embedding []float64
		if err := json.Unmarshal(raw, &embedding); err == nil {
			return embedding, nil
		}
		s.logger.Warn("embedding cache decode falló", slog.String("service", "ollama"), slog.Any("error", err))
		_ = s.cache.Delete(cacheKey)
	} else if !errors.Is(err, cache.ErrCacheMiss) {
		s.logger.Warn("embedding cache read falló", slog.String("service", "ollama"), slog.Any("error", err))
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

	if raw, err := json.Marshal(result.Embedding); err == nil {
		if err := s.cache.Set(cacheKey, raw, s.cacheTTL); err != nil {
			s.logger.Warn("embedding cache write falló", slog.String("service", "ollama"), slog.Any("error", err))
		}
	}

	return result.Embedding, nil
}
