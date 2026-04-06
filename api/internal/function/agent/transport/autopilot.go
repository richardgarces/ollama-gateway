package transport

import (
	"encoding/json"
	"net/http"

	agentservice "ollama-gateway/internal/function/agent"
	"ollama-gateway/pkg/httputil"
)

type AutopilotHandler struct {
	service *agentservice.AutopilotService
}

func NewAutopilotHandler(service *agentservice.AutopilotService) *AutopilotHandler {
	return &AutopilotHandler{service: service}
}

func (h *AutopilotHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task    string                        `json:"task"`
		Model   string                        `json:"model"`
		Context agentservice.WorkspaceContext `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Task == "" {
		httputil.WriteError(w, http.StatusBadRequest, "task requerido")
		return
	}

	if err := httputil.WriteSSEHeaders(w); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "streaming no soportado")
		return
	}

	ctx := r.Context()

	err := h.service.RunStream(ctx, req.Task, req.Model, req.Context, func(ev agentservice.AutopilotEvent) {
		httputil.WriteSSEData(w, ev)
	})
	if err != nil {
		httputil.WriteSSEData(w, agentservice.AutopilotEvent{
			Event:   "error",
			Content: err.Error(),
		})
	}

	httputil.WriteSSEDone(w)
}
