package runtimeconfig

import (
	"fmt"
	"sync"
	"time"

	"ollama-gateway/internal/config"
)

type Service struct {
	cfg *config.Config
	mu  sync.Mutex
}

type ReloadResult struct {
	Port               string    `json:"port"`
	RateLimitRPM       int       `json:"rate_limit_rpm"`
	HTTPTimeoutSeconds int       `json:"http_timeout_seconds"`
	ReloadedAt         time.Time `json:"reloaded_at"`
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) Reload() (*ReloadResult, error) {
	if s == nil || s.cfg == nil {
		return nil, fmt.Errorf("runtime config service no disponible")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next, err := config.LoadWithError()
	if err != nil {
		return nil, err
	}
	*s.cfg = *next

	return &ReloadResult{
		Port:               s.cfg.Port,
		RateLimitRPM:       s.cfg.RateLimitRPM,
		HTTPTimeoutSeconds: s.cfg.HTTPTimeoutSeconds,
		ReloadedAt:         time.Now().UTC(),
	}, nil
}
