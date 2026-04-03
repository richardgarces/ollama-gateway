package transport

import (
	"ollama-gateway/internal/function/core/domain"
)

type Handler = GenerateHandler

func NewHandler(ragService domain.RAGEngine) *Handler {
	return NewGenerateHandler(ragService)
}
