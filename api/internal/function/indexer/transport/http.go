package transport

import (
	"ollama-gateway/internal/function/core/domain"
)

type Handler = IndexerHandler

func NewHandler(indexer domain.Indexer) *Handler {
	return NewIndexerHandler(indexer)
}
