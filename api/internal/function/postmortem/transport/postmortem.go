package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	postmortemservice "ollama-gateway/internal/function/postmortem"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *postmortemservice.Service
}

func NewHandler(svc *postmortemservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Analyze(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "postmortem service no disponible")
		return
	}

	var req postmortemservice.IncidentInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body invalido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Logs) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "logs requeridos")
		return
	}

	report, err := h.svc.AnalyzeIncident(req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, report)
}
