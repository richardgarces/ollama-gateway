package domain

import "time"

type Profile struct {
	UserID         string    `json:"user_id" bson:"user_id"`
	PreferredModel string    `json:"preferred_model" bson:"preferred_model"`
	Temperature    float64   `json:"temperature" bson:"temperature"`
	SystemPrompt   string    `json:"system_prompt" bson:"system_prompt"`
	MaxTokens      int       `json:"max_tokens" bson:"max_tokens"`
	CreatedAt      time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" bson:"updated_at"`
}
