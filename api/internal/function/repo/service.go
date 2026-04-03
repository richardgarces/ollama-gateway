package service

import (
	"log/slog"

	coreservice "ollama-gateway/internal/function/core"
)

type Service = RepoService

func NewService(ollamaService *coreservice.OllamaService, repoRoot string, logger *slog.Logger) *Service {
	return NewRepoService(ollamaService, repoRoot, logger)
}
