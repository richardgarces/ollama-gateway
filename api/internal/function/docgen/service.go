package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = DocGenService

func NewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *Service {
	return NewDocGenService(rag, repoRoot, logger)
}
