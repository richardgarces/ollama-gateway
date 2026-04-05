package transport

import (
	"ollama-gateway/internal/utils/observability"
)

type Handler = MetricsHandler

type Collector interface {
	Snapshot() observability.MetricsSnapshot
	ValueSnapshot() observability.ValueMetricsSnapshot
	TraceFeaturesSnapshot() observability.FeatureTraceSnapshot
}

func NewHandler(collector Collector) *Handler {
	return NewMetricsHandler(collector)
}
