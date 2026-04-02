package handlers

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"
)

type AgentHandler struct {
	agentService domain.AgentRunner
}

func NewAgentHandler(agentService domain.AgentRunner) *AgentHandler {
	return &AgentHandler{agentService: agentService}
}

func (h *AgentHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req domain.AgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	if req.Input == "" {
		httputil.WriteError(w, http.StatusBadRequest, "input requerido")
		return
	}

	result := h.agentService.Run(req.Input)
	httputil.WriteJSON(w, http.StatusOK, domain.AgentResponse{Result: result})
}
