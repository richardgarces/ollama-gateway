package service

import (
	"log/slog"

	coreservice "ollama-gateway/internal/function/core"
)

type Service = IndexerService

func NewService(repoRoots []string, statePath string, ollama *coreservice.OllamaService, qdrant *coreservice.QdrantService, logger *slog.Logger) (*Service, error) {
	return NewIndexerService(repoRoots, statePath, ollama, qdrant, logger)
}
