package service

import (
	"log/slog"

	"ollama-gateway/internal/function/core/domain"
)

type Service = ReviewService

func NewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *Service {
	return NewReviewService(rag, repoRoot, logger)
}
