package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = TranslatorService

func NewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *Service {
	return NewTranslatorService(rag, repoRoot, logger)
}
