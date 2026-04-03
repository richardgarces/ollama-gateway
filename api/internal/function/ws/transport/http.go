package transport

import (
	"ollama-gateway/internal/function/core/domain"
)

type Handler = WSHandler

func NewHandler(rag domain.RAGEngine, jwtSecret []byte) *Handler {
	return NewWSHandler(rag, jwtSecret)
}
