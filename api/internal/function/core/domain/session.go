package domain

import "time"

type ChatSession struct {
	ID           string    `json:"id"`
	OwnerID      string    `json:"owner_id"`
	Participants []string  `json:"participants"`
	Messages     []Message `json:"messages"`
	CreatedAt    time.Time `json:"created_at"`
	IsActive     bool      `json:"is_active"`
}
