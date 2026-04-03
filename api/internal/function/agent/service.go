package service

import (
	"log/slog"

	coreservice "ollama-gateway/internal/function/core"
)

type Service = AgentService

func NewService(ollamaService *coreservice.OllamaService, logger *slog.Logger, toolRegistry *coreservice.ToolRegistry) *Service {
	return NewAgentService(ollamaService, logger, toolRegistry)
}
