package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	outboxservice "ollama-gateway/internal/function/outbox"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *outboxservice.Service
}

type retryRequest struct {
	ID      string `json:"id"`
	AllDead bool   `json:"all_dead"`
}

func NewHandler(svc *outboxservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "outbox service no disponible")
		return
	}

	var req retryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if !req.AllDead && req.ID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "id requerido o all_dead=true")
		return
	}

	count, err := h.svc.RetryDead(r.Context(), req.ID, req.AllDead)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"retried":  count,
		"all_dead": req.AllDead,
		"id":       req.ID,
	})
}
