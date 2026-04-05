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
	ValueSnapshot() observability.ValueMetricsSnapshot
	TraceFeaturesSnapshot() observability.FeatureTraceSnapshot
}

func NewMetricsHandler(collector metricsCollector) *MetricsHandler {
	return &MetricsHandler{collector: collector}
}

func (h *MetricsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, h.collector.Snapshot())
}

func (h *MetricsHandler) Value(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, h.collector.ValueSnapshot())
}

func (h *MetricsHandler) TraceFeatures(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, h.collector.TraceFeaturesSnapshot())
}
