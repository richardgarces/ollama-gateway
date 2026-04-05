package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/internal/function/resilience"
	"ollama-gateway/pkg/cache"
	"ollama-gateway/pkg/httpclient"

	"github.com/shirou/gopsutil/v4/mem"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type OllamaService struct {
	baseURL      string
	client       *http.Client
	cache        cache.Cache
	cacheTTL     time.Duration
	maxCacheSize int
	logger       *slog.Logger
	offline      bool
	chatModel    string
	fimModel     string
	embedModel   string
	keepAlive    string
	autoQuantize bool
	breaker      *resilience.CircuitBreaker
	embeddingSem chan struct{}
	modelsMu     sync.Mutex
	modelsCache  []string
	modelsAt     time.Time
	poolObserver interface {
		RegisterPool(name string, capacity int)
		ObservePoolAcquire(name string, waited bool)
		ObservePoolRelease(name string)
	}
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
	s := &OllamaService{
		baseURL: cfg.OllamaURL,
		client: httpclient.NewResilientClient(httpclient.Options{
			Timeout:             time.Duration(cfg.PoolTimeoutSeconds) * time.Second,
			MaxRetries:          cfg.HTTPMaxRetries,
			MaxConnsPerHost:     cfg.PoolMaxOpen,
			MaxIdleConns:        cfg.PoolMaxOpen,
			MaxIdleConnsPerHost: cfg.PoolMaxIdle,
			IdleConnTimeout:     time.Duration(cfg.PoolTimeoutSeconds) * time.Second,
			DialTimeout:         time.Duration(cfg.PoolTimeoutSeconds) * time.Second,
		}),
		cache:        embeddingCache,
		cacheTTL:     ttl,
		maxCacheSize: max,
		logger:       logger,
		chatModel:    strings.TrimSpace(cfg.ChatModel),
		fimModel:     strings.TrimSpace(cfg.FIMModel),
		embedModel:   strings.TrimSpace(cfg.EmbeddingModel),
		keepAlive:    strings.TrimSpace(cfg.OllamaKeepAlive),
		autoQuantize: cfg.AutoQuantizeModels,
		breaker: resilience.NewCircuitBreaker(resilience.Config{
			Name:               "ollama",
			FailureThreshold:   circuitThreshold(cfg.CBOllamaThreshold, cfg.CBFailureThreshold),
			OpenTimeout:        time.Duration(cfg.CBOpenTimeoutSeconds) * time.Second,
			HalfOpenMaxSuccess: cfg.CBHalfOpenMaxSuccess,
		}),
		embeddingSem: make(chan struct{}, maxPoolSize(cfg.EmbeddingPoolSize, 8)),
	}
	s.initializeAvailability()
	return s
}

func (s *OllamaService) ChatModelName() string {
	if s.chatModel != "" {
		return s.chatModel
	}
	return "phi3:latest"
}

func (s *OllamaService) SetPoolObserver(observer interface {
	RegisterPool(name string, capacity int)
	ObservePoolAcquire(name string, waited bool)
	ObservePoolRelease(name string)
}) {
	if s == nil {
		return
	}
	s.poolObserver = observer
	if observer != nil {
		observer.RegisterPool("embedding", cap(s.embeddingSem))
	}
}

func (s *OllamaService) initializeAvailability() {
	if s.Ping() {
		s.offline = false
		return
	}
	s.offline = true
	s.logger.Warn("Ollama offline - generation disabled")
}

func (s *OllamaService) Ping() bool {
	if s.breaker != nil {
		if snap := s.breaker.Snapshot(); snap.State == resilience.StateOpen {
			if err := s.breaker.Execute(context.Background(), func(context.Context) error { return nil }); err != nil {
				return false
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.baseURL, "/")+"/", nil)
	if err != nil {
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

func (s *OllamaService) ListModels() ([]string, error) {
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	err := s.withBreaker(context.Background(), func(ctx context.Context) error {
		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimRight(s.baseURL, "/")+"/api/tags", nil)
		if reqErr != nil {
			return reqErr
		}
		resp, doErr := s.client.Do(req)
		if doErr != nil {
			return fmt.Errorf("Ollama is not available. Check that the service is running.")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Ollama is not available. Check that the service is running.")
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&out); decodeErr != nil {
			return decodeErr
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	models := make([]string, 0, len(out.Models))
	for _, m := range out.Models {
		if strings.TrimSpace(m.Name) != "" {
			models = append(models, m.Name)
		}
	}
	return models, nil
}

func (s *OllamaService) ensureAvailable() error {
	if s.Ping() {
		s.offline = false
		return nil
	}
	s.offline = true
	return fmt.Errorf("Ollama is not available. Check that the service is running.")
}

func (s *OllamaService) Generate(model, prompt string) (string, error) {
	return s.GenerateWithContext(context.Background(), model, prompt)
}

func (s *OllamaService) GenerateWithContext(ctx context.Context, model, prompt string) (string, error) {
	ctx, span := otel.Tracer("ollama-gateway/service/ollama").Start(ctx, "OllamaService.GenerateWithContext")
	defer span.End()
	span.SetAttributes(
		attribute.String("llm.model.requested", strings.TrimSpace(model)),
		attribute.Int("prompt.length", len(prompt)),
	)

	if err := s.ensureAvailable(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	model = s.effectiveModel(model, false)
	span.SetAttributes(attribute.String("llm.model.effective", model))
	reqBody := domain.OllamaRequest{
		Model:     model,
		Prompt:    prompt,
		Stream:    false,
		KeepAlive: s.keepAlive,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	var result domain.OllamaResponse
	err = s.withBreaker(ctx, func(ctx context.Context) error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/generate", bytes.NewBuffer(data))
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("Content-Type", "application/json")
		resp, postErr := s.client.Do(req)
		if postErr != nil {
			return fmt.Errorf("Ollama is not available. Check that the service is running.")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("ollama returned status %d", resp.StatusCode)
		}

		if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil {
			return fmt.Errorf("ollama decode error: %w", decodeErr)
		}
		return nil
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	span.SetStatus(codes.Ok, "")
	return result.Response, nil
}

func (s *OllamaService) StreamGenerate(model, prompt string, onChunk func(string) error) error {
	if err := s.ensureAvailable(); err != nil {
		return err
	}
	model = s.effectiveModel(model, true)
	reqBody := domain.OllamaRequest{
		Model:     model,
		Prompt:    prompt,
		Stream:    true,
		KeepAlive: s.keepAlive,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	return s.withBreaker(context.Background(), func(ctx context.Context) error {
		resp, postErr := s.client.Post(s.baseURL+"/api/generate", "application/json", bytes.NewBuffer(data))
		if postErr != nil {
			return fmt.Errorf("Ollama is not available. Check that the service is running.")
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
			if decodeErr := decoder.Decode(&chunk); decodeErr != nil {
				if decodeErr == io.EOF {
					return nil
				}
				return fmt.Errorf("ollama streaming decode error: %w", decodeErr)
			}
			if chunk.Response != "" {
				if streamErr := onChunk(chunk.Response); streamErr != nil {
					return streamErr
				}
			}
			if chunk.Done {
				return nil
			}
		}
	})
}

func (s *OllamaService) StreamChat(model string, messages []domain.Message, onChunk func(string) error) error {
	if err := s.ensureAvailable(); err != nil {
		return err
	}
	model = s.effectiveModel(model, true)
	reqBody := domain.OllamaChatRequest{
		Model:     model,
		Messages:  messages,
		Stream:    true,
		KeepAlive: s.keepAlive,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	return s.withBreaker(context.Background(), func(ctx context.Context) error {
		resp, postErr := s.client.Post(s.baseURL+"/api/chat", "application/json", bytes.NewBuffer(data))
		if postErr != nil {
			return fmt.Errorf("Ollama is not available. Check that the service is running.")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("ollama returned status %d", resp.StatusCode)
		}

		decoder := json.NewDecoder(resp.Body)
		for {
			var chunk struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Done bool `json:"done"`
			}
			if decodeErr := decoder.Decode(&chunk); decodeErr != nil {
				if decodeErr == io.EOF {
					return nil
				}
				return fmt.Errorf("ollama chat streaming decode error: %w", decodeErr)
			}
			if chunk.Message.Content != "" {
				if streamErr := onChunk(chunk.Message.Content); streamErr != nil {
					return streamErr
				}
			}
			if chunk.Done {
				return nil
			}
		}
	})
}

func (s *OllamaService) GetEmbedding(model, text string) ([]float64, error) {
	ctx, span := otel.Tracer("ollama-gateway/service/ollama").Start(context.Background(), "OllamaService.GetEmbedding")
	defer span.End()
	span.SetAttributes(
		attribute.String("embedding.model.requested", strings.TrimSpace(model)),
		attribute.Int("embedding.text.length", len(text)),
	)

	model = s.effectiveEmbeddingModel(model)
	span.SetAttributes(attribute.String("embedding.model.effective", model))
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

	waited := s.acquireEmbeddingSlot()
	defer s.releaseEmbeddingSlot()
	if s.poolObserver != nil {
		s.poolObserver.ObservePoolAcquire("embedding", waited)
		defer s.poolObserver.ObservePoolRelease("embedding")
	}

	reqBody := map[string]interface{}{
		"model":      model,
		"prompt":     text,
		"keep_alive": s.keepAlive,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	err = s.withBreaker(ctx, func(ctx context.Context) error {
		resp, postErr := s.client.Post(s.baseURL+"/api/embeddings", "application/json", bytes.NewBuffer(data))
		if postErr != nil {
			return fmt.Errorf("Ollama is not available. Check that the service is running.")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("ollama returned status %d", resp.StatusCode)
		}

		if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil {
			return fmt.Errorf("ollama embedding decode error: %w", decodeErr)
		}
		return nil
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	if raw, err := json.Marshal(result.Embedding); err == nil {
		if err := s.cache.Set(cacheKey, raw, s.cacheTTL); err != nil {
			s.logger.Warn("embedding cache write falló", slog.String("service", "ollama"), slog.Any("error", err))
		}
	}

	span.SetStatus(codes.Ok, "")
	return result.Embedding, nil
}

func (s *OllamaService) CircuitBreakerState() resilience.Snapshot {
	if s == nil || s.breaker == nil {
		return resilience.Snapshot{Name: "ollama", State: resilience.StateClosed}
	}
	return s.breaker.Snapshot()
}

func (s *OllamaService) withBreaker(ctx context.Context, op func(context.Context) error) error {
	if s == nil || s.breaker == nil {
		return op(ctx)
	}
	return s.breaker.Execute(ctx, op)
}

func circuitThreshold(providerThreshold, fallback int) int {
	if providerThreshold > 0 {
		return providerThreshold
	}
	if fallback > 0 {
		return fallback
	}
	return 3
}

func maxPoolSize(v, fallback int) int {
	if v > 0 {
		return v
	}
	if fallback > 0 {
		return fallback
	}
	return 8
}

func (s *OllamaService) acquireEmbeddingSlot() bool {
	if s == nil || s.embeddingSem == nil {
		return false
	}
	select {
	case s.embeddingSem <- struct{}{}:
		return false
	default:
		s.embeddingSem <- struct{}{}
		return true
	}
}

func (s *OllamaService) releaseEmbeddingSlot() {
	if s == nil || s.embeddingSem == nil {
		return
	}
	select {
	case <-s.embeddingSem:
	default:
	}
}

var quantSuffixRegex = regexp.MustCompile(`(?i)-q(4|8)[^:]*`)

func (s *OllamaService) effectiveEmbeddingModel(requested string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = s.embedModel
	}
	if requested == "" {
		requested = "nomic-embed-text"
	}
	return s.effectiveModel(requested, false)
}

func (s *OllamaService) effectiveModel(requested string, forFIM bool) string {
	model := strings.TrimSpace(requested)
	if model == "" {
		if forFIM && s.fimModel != "" {
			model = s.fimModel
		} else if s.chatModel != "" {
			model = s.chatModel
		}
	}
	if model == "" {
		model = "phi3:latest"
	}
	if !s.autoQuantize {
		return model
	}
	if quantSuffixRegex.MatchString(model) {
		return model
	}
	return s.quantizedModelCandidate(model)
}

func (s *OllamaService) quantizedModelCandidate(base string) string {
	models := s.cachedModels()
	if len(models) == 0 {
		return base
	}
	preferQ8 := false
	if vm, err := mem.VirtualMemory(); err == nil {
		preferQ8 = vm.Total >= 24*1024*1024*1024
	}
	needle := "q4"
	if preferQ8 {
		needle = "q8"
	}
	lowerBase := strings.ToLower(strings.TrimSpace(base))
	for _, m := range models {
		lm := strings.ToLower(m)
		if strings.Contains(lm, lowerBase) && strings.Contains(lm, needle) {
			return m
		}
	}
	return base
}

func (s *OllamaService) cachedModels() []string {
	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()
	if len(s.modelsCache) > 0 && time.Since(s.modelsAt) < 2*time.Minute {
		return append([]string(nil), s.modelsCache...)
	}
	models, err := s.ListModels()
	if err != nil || len(models) == 0 {
		return append([]string(nil), s.modelsCache...)
	}
	s.modelsCache = append([]string(nil), models...)
	s.modelsAt = time.Now()
	return append([]string(nil), s.modelsCache...)
}
