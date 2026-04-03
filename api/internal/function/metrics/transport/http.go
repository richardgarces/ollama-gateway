package transport

import (
	"ollama-gateway/internal/utils/observability"
)

type Handler = MetricsHandler

type Collector interface {
	Snapshot() observability.MetricsSnapshot
}

func NewHandler(collector Collector) *Handler {
	return NewMetricsHandler(collector)
}
