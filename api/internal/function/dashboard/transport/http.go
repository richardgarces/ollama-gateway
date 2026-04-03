package transport

import (
	"ollama-gateway/internal/config"
	"ollama-gateway/internal/utils/observability"
)

type Handler = DashboardHandler

type MetricsCollector interface {
	Snapshot() observability.MetricsSnapshot
}

type IndexerStatus interface {
	Status() map[string]interface{}
}

func NewHandler(cfg *config.Config, metrics MetricsCollector, indexer IndexerStatus, logs *observability.LogStream) *Handler {
	return NewDashboardHandler(cfg, metrics, indexer, logs)
}
