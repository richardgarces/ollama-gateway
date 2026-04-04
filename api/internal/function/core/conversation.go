package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/internal/function/pooling"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	_ "modernc.org/sqlite"
)

type ConversationService struct {
	client     *mongo.Client
	collection *mongo.Collection
	sqliteDB   *sql.DB
	mu         sync.Mutex
	logger     *slog.Logger
}

var ErrConversationNotFound = errors.New("conversation not found")

type mongoConversation struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	UserID    string             `bson:"user_id"`
	Messages  []domain.Message   `bson:"messages"`
	CreatedAt time.Time          `bson:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"`
}

func NewConversationService(mongoURI string, logger *slog.Logger) (*ConversationService, error) {
	return NewConversationServiceWithPool(mongoURI, 0, 0, 5, logger)
}

func NewConversationServiceWithPool(mongoURI string, maxOpen, maxIdle, timeoutSeconds int, logger *slog.Logger) (*ConversationService, error) {
	if logger == nil {
		logger = slog.Default()
	}

	client, err := pooling.ConnectMongo(mongoURI, maxOpen, maxIdle, timeoutSeconds)
	if err != nil {
		return nil, err
	}

	collection := client.Database("ollama_gateway").Collection("conversations")

	svc := &ConversationService{
		client:     client,
		collection: collection,
		logger:     logger,
	}
	return svc, nil
}

func NewConversationServiceSQLite(dbPath string, logger *slog.Logger) (*ConversationService, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if dbPath == "" {
		return nil, errors.New("sqlite db path requerido")
	}
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", absPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		messages_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &ConversationService{sqliteDB: db, logger: logger}, nil
}

func (s *ConversationService) Disconnect(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.client != nil {
		if err := s.client.Disconnect(ctx); err != nil {
			return err
		}
	}
	if s.sqliteDB != nil {
		return s.sqliteDB.Close()
	}
	return nil
}

func (s *ConversationService) Create(ctx context.Context, userID string, messages []domain.Message) (*domain.Conversation, error) {
	if s.sqliteDB != nil {
		return s.createSQLite(ctx, userID, messages)
	}
	now := time.Now().UTC()
	doc := mongoConversation{
		UserID:    userID,
		Messages:  append([]domain.Message(nil), messages...),
		CreatedAt: now,
		UpdatedAt: now,
	}

	result, err := s.collection.InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}

	objID, ok := result.InsertedID.(primitive.ObjectID)
	if !ok {
		return nil, errors.New("inserted id inválido")
	}
	doc.ID = objID
	return toDomainConversation(doc), nil
}

func (s *ConversationService) Append(ctx context.Context, conversationID, userID string, messages []domain.Message) (*domain.Conversation, error) {
	if s.sqliteDB != nil {
		return s.appendSQLite(ctx, conversationID, userID, messages)
	}
	objID, err := primitive.ObjectIDFromHex(conversationID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	filter := bson.M{"_id": objID, "user_id": userID}
	update := bson.M{
		"$push": bson.M{"messages": bson.M{"$each": messages}},
		"$set":  bson.M{"updated_at": now},
	}

	result := s.collection.FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After))
	if err := result.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	var doc mongoConversation
	if err := result.Decode(&doc); err != nil {
		return nil, err
	}
	return toDomainConversation(doc), nil
}

func (s *ConversationService) GetByID(ctx context.Context, conversationID, userID string) (*domain.Conversation, error) {
	if s.sqliteDB != nil {
		return s.getByIDSQLite(ctx, conversationID, userID)
	}
	objID, err := primitive.ObjectIDFromHex(conversationID)
	if err != nil {
		return nil, err
	}

	filter := bson.M{"_id": objID, "user_id": userID}
	var doc mongoConversation
	if err := s.collection.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}
	return toDomainConversation(doc), nil
}

func (s *ConversationService) ListByUser(ctx context.Context, userID string, limit int) ([]domain.Conversation, error) {
	if s.sqliteDB != nil {
		return s.listByUserSQLite(ctx, userID, limit)
	}
	if limit <= 0 {
		limit = 20
	}

	opts := options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(int64(limit))
	cursor, err := s.collection.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	out := make([]domain.Conversation, 0)
	for cursor.Next(ctx) {
		var doc mongoConversation
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		conv := toDomainConversation(doc)
		if conv != nil {
			out = append(out, *conv)
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func toDomainConversation(doc mongoConversation) *domain.Conversation {
	return &domain.Conversation{
		ID:        doc.ID.Hex(),
		UserID:    doc.UserID,
		Messages:  append([]domain.Message(nil), doc.Messages...),
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}
}

var _ domain.ConversationStore = (*ConversationService)(nil)

func (s *ConversationService) createSQLite(ctx context.Context, userID string, messages []domain.Message) (*domain.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	id := fmt.Sprintf("sqlconv-%d", now.UnixNano())
	b, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	_, err = s.sqliteDB.ExecContext(ctx,
		`INSERT INTO conversations(id, user_id, messages_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, userID, string(b), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	return &domain.Conversation{ID: id, UserID: userID, Messages: append([]domain.Message(nil), messages...), CreatedAt: now, UpdatedAt: now}, nil
}

func (s *ConversationService) appendSQLite(ctx context.Context, conversationID, userID string, messages []domain.Message) (*domain.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conv, err := s.getByIDSQLiteUnlocked(ctx, conversationID, userID)
	if err != nil {
		return nil, err
	}
	conv.Messages = append(conv.Messages, messages...)
	conv.UpdatedAt = time.Now().UTC()
	b, err := json.Marshal(conv.Messages)
	if err != nil {
		return nil, err
	}
	_, err = s.sqliteDB.ExecContext(ctx,
		`UPDATE conversations SET messages_json = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		string(b), conv.UpdatedAt.Format(time.RFC3339Nano), conversationID, userID,
	)
	if err != nil {
		return nil, err
	}
	return conv, nil
}

func (s *ConversationService) getByIDSQLite(ctx context.Context, conversationID, userID string) (*domain.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getByIDSQLiteUnlocked(ctx, conversationID, userID)
}

func (s *ConversationService) getByIDSQLiteUnlocked(ctx context.Context, conversationID, userID string) (*domain.Conversation, error) {
	row := s.sqliteDB.QueryRowContext(ctx,
		`SELECT id, user_id, messages_json, created_at, updated_at FROM conversations WHERE id = ? AND user_id = ?`,
		conversationID, userID,
	)
	var id, uid, msgJSON, createdRaw, updatedRaw string
	if err := row.Scan(&id, &uid, &msgJSON, &createdRaw, &updatedRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}
	var msgs []domain.Message
	if err := json.Unmarshal([]byte(msgJSON), &msgs); err != nil {
		msgs = []domain.Message{}
	}
	createdAt, _ := time.Parse(time.RFC3339Nano, createdRaw)
	updatedAt, _ := time.Parse(time.RFC3339Nano, updatedRaw)
	return &domain.Conversation{ID: id, UserID: uid, Messages: msgs, CreatedAt: createdAt, UpdatedAt: updatedAt}, nil
}

func (s *ConversationService) listByUserSQLite(ctx context.Context, userID string, limit int) ([]domain.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.sqliteDB.QueryContext(ctx,
		`SELECT id, user_id, messages_json, created_at, updated_at FROM conversations WHERE user_id = ? ORDER BY updated_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Conversation, 0, limit)
	for rows.Next() {
		var id, uid, msgJSON, createdRaw, updatedRaw string
		if err := rows.Scan(&id, &uid, &msgJSON, &createdRaw, &updatedRaw); err != nil {
			return nil, err
		}
		var msgs []domain.Message
		_ = json.Unmarshal([]byte(msgJSON), &msgs)
		createdAt, _ := time.Parse(time.RFC3339Nano, createdRaw)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedRaw)
		out = append(out, domain.Conversation{ID: id, UserID: uid, Messages: msgs, CreatedAt: createdAt, UpdatedAt: updatedAt})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
