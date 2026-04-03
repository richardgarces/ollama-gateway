package transport

import (
	"ollama-gateway/internal/config"
)

type Handler = HealthHandler

func NewHandler(cfg *config.Config) *Handler {
	return NewHealthHandler(cfg)
}
