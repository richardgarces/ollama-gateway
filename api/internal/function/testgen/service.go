package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = TestGenService

func NewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *Service {
	return NewTestGenService(rag, repoRoot, logger)
}
