package domain

import (
	"context"
	"time"
)

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

type CICDService interface {
	GeneratePipeline(platform, repoRoot string) (string, error)
	OptimizePipeline(existing string, platform string) (string, error)
	ApplyPipeline(platform, repoRoot, content string) (string, string, error)
}

type SessionService interface {
	Create(ownerID string) (*ChatSession, error)
	Join(sessionID, userID string) error
	AddMessage(sessionID, userID string, msg Message) error
	GetMessages(sessionID, userID string, since time.Time) ([]Message, error)
	SetParticipantRole(sessionID, actorID, targetUserID, role string) error
	GetSession(sessionID string) (*ChatSession, error)
}

type DocGenService interface {
	GenerateDocForFile(path string) (string, error)
	GenerateREADME(repoRoot string) (string, error)
	WriteWithBackup(path string, content string) (string, error)
}

type TestGenService interface {
	GenerateTests(code, lang string) (string, error)
	GenerateTestsForFile(path string) (string, error)
	WriteTestsForFile(sourcePath, testCode string) (string, string, error)
}

type CommitGenService interface {
	GenerateMessage(diff string) (string, error)
	GenerateFromStaged(repoRoot string) (string, error)
}

type DebugService interface {
	AnalyzeError(stackTrace string) (DebugAnalysis, error)
	AnalyzeLog(logLines string) (DebugAnalysis, error)
}

type TranslatorService interface {
	Translate(code, fromLang, toLang string) (string, error)
	TranslateFile(path, toLang string) (string, error)
}

type PatchService interface {
	ExtractCodeBlocks(response string) []CodeBlock
	ExtractDiff(response string) []UnifiedDiff
	ApplyPatch(repoRoot string, diff UnifiedDiff) error
}

type RepoService interface {
	Refactor(absPath string) (string, error)
	AnalyzeRepo() (string, error)
}

type ArchitectService interface {
	AnalyzeProject() (ArchReport, error)
	SuggestRefactor(path string) (string, error)
}

type SQLGenService interface {
	GenerateQuery(description, dialect string) (string, error)
	GenerateMigration(description, dialect string) (string, error)
	ExplainQuery(sql string) (string, error)
}
