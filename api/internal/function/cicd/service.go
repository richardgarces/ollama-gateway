package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = CICDService

func NewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *Service {
	return NewCICDService(rag, repoRoot, logger)
}
