package transport

import (
	"ollama-gateway/internal/function/core/domain"
)

type Handler = ChatHandler

func NewHandler(rag domain.RAGEngine) *Handler {
	return NewChatHandler(rag)
}
