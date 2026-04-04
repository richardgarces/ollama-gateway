package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	cachepkg "ollama-gateway/pkg/cache"
)

type MetricsObserver interface {
	ObserveCacheHit()
	ObserveCacheMiss()
}

type Entry struct {
	Retrieval string    `json:"retrieval"`
	Response  string    `json:"response"`
	StoredAt  time.Time `json:"stored_at"`
}

type Service struct {
	backend cachepkg.Cache
	logger  *slog.Logger
	ttl     time.Duration
	prefix  string
	metrics MetricsObserver

	versionMu sync.RWMutex
	versions  map[string]string
}

func NewService(backend cachepkg.Cache, ttl time.Duration, prefix string, logger *slog.Logger, metrics MetricsObserver) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "rag-cache"
	}
	return &Service{
		backend:  backend,
		logger:   logger,
		ttl:      ttl,
		prefix:   prefix,
		metrics:  metrics,
		versions: map[string]string{},
	}
}

func (s *Service) Get(ctx context.Context, repoScope, prompt, contextText string) (Entry, bool, error) {
	if s == nil || s.backend == nil {
		return Entry{}, false, nil
	}
	key, err := s.responseKey(ctx, repoScope, prompt, contextText)
	if err != nil {
		return Entry{}, false, err
	}
	b, err := s.backend.Get(key)
	if err == cachepkg.ErrCacheMiss {
		s.observeMiss()
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	var out Entry
	if err := json.Unmarshal(b, &out); err != nil {
		s.observeMiss()
		return Entry{}, false, nil
	}
	s.observeHit()
	return out, true, nil
}

func (s *Service) Set(ctx context.Context, repoScope, prompt, contextText string, entry Entry) error {
	if s == nil || s.backend == nil {
		return nil
	}
	key, err := s.responseKey(ctx, repoScope, prompt, contextText)
	if err != nil {
		return err
	}
	entry.StoredAt = time.Now().UTC()
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.backend.Set(key, payload, s.ttl)
}

func (s *Service) GetRetrieval(ctx context.Context, repoScope, prompt string) (string, bool, error) {
	if s == nil || s.backend == nil {
		return "", false, nil
	}
	key, err := s.retrievalKey(ctx, repoScope, prompt)
	if err != nil {
		return "", false, err
	}
	b, err := s.backend.Get(key)
	if err == cachepkg.ErrCacheMiss {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

func (s *Service) SetRetrieval(ctx context.Context, repoScope, prompt, retrieval string) error {
	if s == nil || s.backend == nil {
		return nil
	}
	key, err := s.retrievalKey(ctx, repoScope, prompt)
	if err != nil {
		return err
	}
	return s.backend.Set(key, []byte(retrieval), s.ttl)
}

func (s *Service) InvalidateRepo(ctx context.Context, repoScope string) error {
	if s == nil || s.backend == nil {
		return nil
	}
	repoScope = normalizeRepoScope(repoScope)
	newVersion := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.backend.Set(s.versionKey(repoScope), []byte(newVersion), 0); err != nil {
		return err
	}
	s.versionMu.Lock()
	s.versions[repoScope] = newVersion
	s.versionMu.Unlock()
	return nil
}

func (s *Service) responseKey(ctx context.Context, repoScope, prompt, contextText string) (string, error) {
	repoVersion, err := s.repoVersion(ctx, repoScope)
	if err != nil {
		return "", err
	}
	h := hashNormalized(prompt + "|" + contextText)
	return s.prefix + ":response:" + normalizeRepoScope(repoScope) + ":" + repoVersion + ":" + h, nil
}

func (s *Service) retrievalKey(ctx context.Context, repoScope, prompt string) (string, error) {
	repoVersion, err := s.repoVersion(ctx, repoScope)
	if err != nil {
		return "", err
	}
	h := hashNormalized(prompt)
	return s.prefix + ":retrieval:" + normalizeRepoScope(repoScope) + ":" + repoVersion + ":" + h, nil
}

func (s *Service) repoVersion(ctx context.Context, repoScope string) (string, error) {
	repoScope = normalizeRepoScope(repoScope)
	s.versionMu.RLock()
	if v, ok := s.versions[repoScope]; ok && v != "" {
		s.versionMu.RUnlock()
		return v, nil
	}
	s.versionMu.RUnlock()

	key := s.versionKey(repoScope)
	b, err := s.backend.Get(key)
	if err == cachepkg.ErrCacheMiss {
		v := "v1"
		if setErr := s.backend.Set(key, []byte(v), 0); setErr != nil {
			return "", setErr
		}
		s.versionMu.Lock()
		s.versions[repoScope] = v
		s.versionMu.Unlock()
		return v, nil
	}
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		v = "v1"
	}
	s.versionMu.Lock()
	s.versions[repoScope] = v
	s.versionMu.Unlock()
	return v, nil
}

func (s *Service) versionKey(repoScope string) string {
	return s.prefix + ":version:" + normalizeRepoScope(repoScope)
}

func normalizeRepoScope(repoScope string) string {
	repoScope = strings.TrimSpace(repoScope)
	if repoScope == "" {
		return "global"
	}
	repoScope = strings.ReplaceAll(repoScope, " ", "_")
	repoScope = strings.ReplaceAll(repoScope, "/", "_")
	repoScope = strings.ReplaceAll(repoScope, "\\", "_")
	return repoScope
}

func hashNormalized(v string) string {
	normalized := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(v))), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func (s *Service) observeHit() {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveCacheHit()
}

func (s *Service) observeMiss() {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveCacheMiss()
}
