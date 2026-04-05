package transport

import (
	"net/http"

	perfservice "ollama-gateway/internal/function/perf"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *perfservice.Service
}

func NewHandler(svc *perfservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) AnalyzeEndpoints(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "perf service no disponible")
		return
	}

	result, err := h.svc.AnalyzeEndpoints()
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}
