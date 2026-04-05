package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	prsummaryservice "ollama-gateway/internal/function/prsummary"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *prsummaryservice.Service
}

type summarizeRequest struct {
	Diff string `json:"diff"`
}

func NewHandler(svc *prsummaryservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Summarize(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "prsummary service no disponible")
		return
	}

	var req summarizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body invalido: "+err.Error())
		return
	}
	req.Diff = strings.TrimSpace(req.Diff)
	if req.Diff == "" {
		httputil.WriteError(w, http.StatusBadRequest, "diff es requerido")
		return
	}

	result, err := h.svc.SummarizeDiff(req.Diff)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}
