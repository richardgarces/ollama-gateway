package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ollama-gateway/internal/function/core/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ConversationService struct {
	client     *mongo.Client
	collection *mongo.Collection
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
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
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

func (s *ConversationService) Disconnect(ctx context.Context) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Disconnect(ctx)
}

func (s *ConversationService) Create(ctx context.Context, userID string, messages []domain.Message) (*domain.Conversation, error) {
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
