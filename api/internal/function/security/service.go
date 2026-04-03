package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = SecurityService

func NewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *Service {
	return NewSecurityService(rag, repoRoot, logger)
}
