package transport

import (
	"net/http"

	guardrailsservice "ollama-gateway/internal/function/guardrails"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *guardrailsservice.Service
}

func NewHandler(svc *guardrailsservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Rules(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "guardrails service no disponible")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"rules": h.svc.Rules(),
	})
}
