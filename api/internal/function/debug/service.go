package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = DebugService

func NewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *Service {
	return NewDebugService(rag, repoRoot, logger)
}
