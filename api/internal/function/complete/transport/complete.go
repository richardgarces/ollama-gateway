package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	completeservice "ollama-gateway/internal/function/complete"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *completeservice.Service
}

func NewHandler(svc *completeservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "complete service no disponible")
		return
	}

	var req completeservice.CompleteInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	res, err := h.svc.Complete(ctx, req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "disponible") {
			status = http.StatusServiceUnavailable
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusRequestTimeout
		}
		httputil.WriteError(w, status, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"completion": res.Completion,
		"model":      res.Model,
	})
}
