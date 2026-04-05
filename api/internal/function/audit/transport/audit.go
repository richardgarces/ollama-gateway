package transport

import (
	"net/http"
	"strconv"
	"strings"

	auditservice "ollama-gateway/internal/function/audit"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *auditservice.Service
}

func NewHandler(svc *auditservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "audit service no disponible")
		return
	}

	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			limit = v
		}
	}
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	result := strings.TrimSpace(r.URL.Query().Get("result"))
	events := h.svc.List(limit, action, result)

	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "csv") {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=audit-events.csv")
		if err := auditservice.WriteCSV(w, events); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"count":  len(events),
		"events": events,
	})
}
