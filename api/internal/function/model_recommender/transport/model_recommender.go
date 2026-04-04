package transport

import (
	"encoding/json"
	"net/http"

	modelrecommenderservice "ollama-gateway/internal/function/model_recommender"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *modelrecommenderservice.Service
}

type recommendRequest struct {
	TaskType     string `json:"task_type"`
	SLALatencyMS int    `json:"sla_latency_ms"`
	TokenBudget  int    `json:"token_budget"`
}

func NewHandler(svc *modelrecommenderservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Recommend(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "model recommender no disponible")
		return
	}

	var req recommendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	rec := h.svc.Recommend(req.TaskType, req.SLALatencyMS, req.TokenBudget)
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"task_type":      req.TaskType,
		"sla_latency_ms": req.SLALatencyMS,
		"token_budget":   req.TokenBudget,
		"model":          rec.Model,
		"score":          rec.Score,
		"explanation":    rec.Explanation,
	})
}
