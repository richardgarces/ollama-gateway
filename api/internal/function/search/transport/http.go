package transport

import (
	"ollama-gateway/internal/function/core/domain"
)

type Handler = SearchHandler

func NewHandler(ollama domain.OllamaClient, vectorStore domain.VectorStore, repos []string) *Handler {
	return NewSearchHandler(ollama, vectorStore, repos)
}
