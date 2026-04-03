package domain

import "time"

type Conversation struct {
	ID        string    `json:"id" bson:"-"`
	UserID    string    `json:"user_id" bson:"user_id"`
	Messages  []Message `json:"messages" bson:"messages"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}
