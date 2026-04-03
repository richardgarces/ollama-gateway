package transport

import (
	"ollama-gateway/internal/function/core/domain"
	sessionsvc "ollama-gateway/internal/function/session"
)

type Handler = SessionHandler

func NewHandler(sessions *sessionsvc.Service, rag domain.RAGEngine) *Handler {
	return NewSessionHandler(sessions, rag)
}
