package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = CommitGenService

func NewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *Service {
	return NewCommitGenService(rag, repoRoot, logger)
}
