package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	testintelservice "ollama-gateway/internal/function/testintel"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *testintelservice.Service
}

type request struct {
	Diff string `json:"diff"`
}

func NewHandler(svc *testintelservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Prioritize(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "test intelligence service no disponible")
		return
	}

	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body JSON invalido")
		return
	}
	if strings.TrimSpace(req.Diff) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "diff es requerido")
		return
	}

	report := h.svc.AnalyzeDiff(testintelservice.AnalyzeInput{Diff: req.Diff})
	httputil.WriteJSON(w, http.StatusOK, report)
}
