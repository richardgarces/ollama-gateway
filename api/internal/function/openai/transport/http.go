package transport

import (
	"ollama-gateway/internal/function/core/domain"
)

type Handler = OpenAIHandler

func NewHandler(
	o domain.OllamaClient,
	r domain.RAGEngine,
	c domain.ConversationStore,
	p domain.ProfileStore,
) *Handler {
	return NewOpenAIHandler(o, r, c, p)
}
