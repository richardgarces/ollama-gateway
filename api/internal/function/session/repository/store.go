package repository

import (
	"time"

	basedomain "ollama-gateway/internal/function/core/domain"
)

type Store interface {
	Create(ownerID string) (*basedomain.ChatSession, error)
	Join(sessionID, userID string) error
	AddMessage(sessionID string, msg basedomain.Message) error
	GetMessages(sessionID string, since time.Time) ([]basedomain.Message, error)
}
