package transport

import (
	"net/http"

	"ollama-gateway/internal/utils/observability"
	"ollama-gateway/pkg/httputil"
)

type MetricsHandler struct {
	collector metricsCollector
}

type metricsCollector interface {
	Snapshot() observability.MetricsSnapshot
}

func NewMetricsHandler(collector metricsCollector) *MetricsHandler {
	return &MetricsHandler{collector: collector}
}

func (h *MetricsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, h.collector.Snapshot())
}
