package services

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ollama-gateway/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProfileService struct {
	client     *mongo.Client
	collection *mongo.Collection
	logger     *slog.Logger
}

var ErrProfileNotFound = errors.New("profile not found")

func NewProfileService(mongoURI string, logger *slog.Logger) (*ProfileService, error) {
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

	collection := client.Database("ollama_gateway").Collection("profiles")
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	return &ProfileService{client: client, collection: collection, logger: logger}, nil
}

func (s *ProfileService) Disconnect(ctx context.Context) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Disconnect(ctx)
}

func (s *ProfileService) Create(ctx context.Context, profile domain.Profile) (*domain.Profile, error) {
	if profile.UserID == "" {
		return nil, errors.New("user_id requerido")
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	setDefaults(&profile)

	if _, err := s.collection.InsertOne(ctx, profile); err != nil {
		return nil, err
	}
	out := profile
	return &out, nil
}

func (s *ProfileService) GetByUserID(ctx context.Context, userID string) (*domain.Profile, error) {
	var profile domain.Profile
	if err := s.collection.FindOne(ctx, bson.M{"user_id": userID}).Decode(&profile); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrProfileNotFound
		}
		return nil, err
	}
	setDefaults(&profile)
	return &profile, nil
}

func (s *ProfileService) Update(ctx context.Context, profile domain.Profile) (*domain.Profile, error) {
	if profile.UserID == "" {
		return nil, errors.New("user_id requerido")
	}
	current, err := s.GetByUserID(ctx, profile.UserID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	profile.CreatedAt = current.CreatedAt
	profile.UpdatedAt = now
	setDefaults(&profile)

	res := s.collection.FindOneAndReplace(ctx,
		bson.M{"user_id": profile.UserID},
		profile,
		options.FindOneAndReplace().SetReturnDocument(options.After),
	)
	if err := res.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrProfileNotFound
		}
		return nil, err
	}
	var updated domain.Profile
	if err := res.Decode(&updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *ProfileService) Upsert(ctx context.Context, profile domain.Profile) (*domain.Profile, error) {
	if profile.UserID == "" {
		return nil, errors.New("user_id requerido")
	}

	now := time.Now().UTC()
	setDefaults(&profile)

	update := bson.M{
		"$set": bson.M{
			"preferred_model": profile.PreferredModel,
			"temperature":     profile.Temperature,
			"system_prompt":   profile.SystemPrompt,
			"max_tokens":      profile.MaxTokens,
			"updated_at":      now,
		},
		"$setOnInsert": bson.M{
			"user_id":    profile.UserID,
			"created_at": now,
		},
	}

	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	res := s.collection.FindOneAndUpdate(ctx, bson.M{"user_id": profile.UserID}, update, opts)
	if err := res.Err(); err != nil {
		return nil, err
	}
	var out domain.Profile
	if err := res.Decode(&out); err != nil {
		return nil, err
	}
	setDefaults(&out)
	return &out, nil
}

func (s *ProfileService) Delete(ctx context.Context, userID string) error {
	result, err := s.collection.DeleteOne(ctx, bson.M{"user_id": userID})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrProfileNotFound
	}
	return nil
}

func setDefaults(profile *domain.Profile) {
	if profile == nil {
		return
	}
	if profile.PreferredModel == "" {
		profile.PreferredModel = "local-rag"
	}
	if profile.Temperature <= 0 {
		profile.Temperature = 0.7
	}
	if profile.MaxTokens <= 0 {
		profile.MaxTokens = 1024
	}
}

var _ domain.ProfileStore = (*ProfileService)(nil)
