package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = ArchitectService

func NewService(rag domain.RAGEngine, repoRoot string, indexer domain.Indexer, logger *slog.Logger) *Service {
	return NewArchitectService(rag, repoRoot, indexer, logger)
}
