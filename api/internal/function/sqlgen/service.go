package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = SQLGenService

func NewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *Service {
	return NewSQLGenService(rag, repoRoot, logger)
}
