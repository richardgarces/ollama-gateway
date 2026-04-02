package domain

import "context"

type OllamaClient interface {
	Generate(model, prompt string) (string, error)
	StreamGenerate(model, prompt string, onChunk func(string) error) error
	GetEmbedding(model, text string) ([]float64, error)
}

type VectorStore interface {
	UpsertPoint(collection, id string, vector []float64, payload map[string]interface{}) error
	Search(collection string, vector []float64, limit int) (map[string]interface{}, error)
}

type RAGEngine interface {
	GenerateWithContext(prompt string) (string, error)
	StreamGenerateWithContext(prompt string, onChunk func(string) error) error
}

type Indexer interface {
	IndexRepo() error
	StartWatcher() error
	StopWatcher()
	ClearState() error
}

type AgentRunner interface {
	Run(prompt string) string
}

type ConversationStore interface {
	Create(ctx context.Context, userID string, messages []Message) (*Conversation, error)
	Append(ctx context.Context, conversationID, userID string, messages []Message) (*Conversation, error)
	GetByID(ctx context.Context, conversationID, userID string) (*Conversation, error)
	ListByUser(ctx context.Context, userID string, limit int) ([]Conversation, error)
}

type ProfileStore interface {
	Create(ctx context.Context, profile Profile) (*Profile, error)
	GetByUserID(ctx context.Context, userID string) (*Profile, error)
	Update(ctx context.Context, profile Profile) (*Profile, error)
	Upsert(ctx context.Context, profile Profile) (*Profile, error)
	Delete(ctx context.Context, userID string) error
}
