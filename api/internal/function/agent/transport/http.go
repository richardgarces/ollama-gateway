package transport

import (
	"ollama-gateway/internal/function/core/domain"
)

type Handler = AgentHandler

func NewHandler(agentService domain.AgentRunner) *Handler {
	return NewAgentHandler(agentService)
}
