package transport

import (
	"net/http"
	"strconv"
	"strings"

	techdebtservice "ollama-gateway/internal/function/techdebt"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *techdebtservice.Service
}

func NewHandler(svc *techdebtservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Priorities(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "techdebt service no disponible")
		return
	}

	report, err := h.svc.AnalyzeTechDebt()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	apply := parseBool(r.URL.Query().Get("apply"))
	reportPath := ""
	if apply {
		reportPath, err = h.svc.WriteReport(report)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"generatedAt": report.GeneratedAt,
		"scanned":     report.Scanned,
		"backlog":     report.Backlog,
		"applied":     apply,
		"reportPath":  reportPath,
	})
}

func parseBool(v string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(v))
	if trimmed == "" {
		return false
	}
	parsed, err := strconv.ParseBool(trimmed)
	if err != nil {
		return trimmed == "1" || trimmed == "yes" || trimmed == "on"
	}
	return parsed
}
