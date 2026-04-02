package handlers

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"
)

type AgentHandler struct {
	agentService interface {
		Run(prompt string) string
	}
}

func NewAgentHandler(agentService interface {
	Run(prompt string) string
}) *AgentHandler {
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
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/domain"
	"ollama-gateway/internal/services"
	"ollama-gateway/pkg/httputil"
)

type AgentHandler struct {
	agentService *services.AgentService
}

func NewAgentHandler(agentService *services.AgentService) *AgentHandler {
	return &AgentHandler{agentService: agentService}
}

func (h *AgentHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req domain.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Prompt == "" {
		httputil.WriteError(w, http.StatusBadRequest, "prompt requerido")
		return
	}

	result := h.agentService.Execute(req.Prompt)
	httputil.WriteJSON(w, http.StatusOK, domain.Response{Result: result})
}
