package transport

import (
	"encoding/json"
	"net/http"

	plannerservice "ollama-gateway/internal/function/planner"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *plannerservice.Service
}

type executePlanRequest struct {
	Steps []plannerservice.Step `json:"steps"`
}

func NewHandler(svc *plannerservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ExecutePlan(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "planner service no disponible")
		return
	}

	var req executePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if len(req.Steps) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "steps requeridos")
		return
	}

	result := h.svc.ExecutePlan(req.Steps)
	status := http.StatusOK
	if result.Status == plannerservice.StepStatusFailed {
		status = http.StatusUnprocessableEntity
	}
	httputil.WriteJSON(w, status, result)
}
