package handlers

import (
	"net/http"

	"ollama-gateway/internal/observability"
	"ollama-gateway/pkg/httputil"
)

type MetricsHandler struct {
	collector interface {
		Snapshot() observability.MetricsSnapshot
	}
}

func NewMetricsHandler(collector interface {
	Snapshot() observability.MetricsSnapshot
}) *MetricsHandler {
	return &MetricsHandler{collector: collector}
}

func (h *MetricsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, h.collector.Snapshot())
}
