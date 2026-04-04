package service

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	outboxservice "ollama-gateway/internal/function/outbox"
	"ollama-gateway/internal/function/pooling"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Service struct {
	client     *mongo.Client
	collection *mongo.Collection
	outbox     *mongo.Collection
	logger     *slog.Logger
}

type SaveFeedbackInput struct {
	Rating    int                    `json:"rating"`
	Comment   string                 `json:"comment"`
	RequestID string                 `json:"request_id"`
	Model     string                 `json:"model"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type FeedbackRecord struct {
	ID        string                 `json:"id" bson:"_id,omitempty"`
	Rating    int                    `json:"rating" bson:"rating"`
	Comment   string                 `json:"comment,omitempty" bson:"comment,omitempty"`
	RequestID string                 `json:"request_id,omitempty" bson:"request_id,omitempty"`
	Model     string                 `json:"model" bson:"model"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" bson:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at" bson:"created_at"`
}

type ModelFeedbackSummary struct {
	Model         string  `json:"model"`
	Count         int     `json:"count"`
	AverageRating float64 `json:"average_rating"`
	Score         float64 `json:"score"`
}

type FeedbackSummary struct {
	WindowHours   int                    `json:"window_hours"`
	TotalCount    int                    `json:"total_count"`
	AverageRating float64                `json:"average_rating"`
	ByModel       []ModelFeedbackSummary `json:"by_model"`
}

func NewService(mongoURI string, logger *slog.Logger) (*Service, error) {
	return NewServiceWithPool(mongoURI, 0, 0, 5, logger)
}

func NewServiceWithPool(mongoURI string, maxOpen, maxIdle, timeoutSeconds int, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(mongoURI) == "" {
		return nil, errors.New("mongo uri requerido")
	}

	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	client, err := pooling.ConnectMongo(mongoURI, maxOpen, maxIdle, timeoutSeconds)
	if err != nil {
		return nil, err
	}

	collection := client.Database("ollama_gateway").Collection("feedback")
	outboxCollection := client.Database("ollama_gateway").Collection("outbox_events")
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: -1}}})
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "model", Value: 1}, {Key: "created_at", Value: -1}}})
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "request_id", Value: 1}}})
	_, _ = outboxCollection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "next_attempt_at", Value: 1}}})
	_, _ = outboxCollection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: -1}}})

	return &Service{client: client, collection: collection, outbox: outboxCollection, logger: logger}, nil
}

func (s *Service) Disconnect(ctx context.Context) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Disconnect(ctx)
}

func (s *Service) SaveFeedback(ctx context.Context, in SaveFeedbackInput) (*FeedbackRecord, error) {
	if s == nil || s.collection == nil {
		return nil, errors.New("feedback service no disponible")
	}
	rating := in.Rating
	if rating < 1 || rating > 5 {
		return nil, errors.New("rating debe estar entre 1 y 5")
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		if v, ok := in.Metadata["model"]; ok {
			if m, ok := v.(string); ok {
				model = strings.TrimSpace(m)
			}
		}
	}
	if model == "" {
		model = "unknown"
	}

	rec := &FeedbackRecord{
		Rating:    rating,
		Comment:   strings.TrimSpace(in.Comment),
		RequestID: strings.TrimSpace(in.RequestID),
		Model:     model,
		Metadata:  in.Metadata,
		CreatedAt: time.Now().UTC(),
	}

	outboxPayload := map[string]interface{}{
		"rating":     rec.Rating,
		"comment":    rec.Comment,
		"request_id": rec.RequestID,
		"model":      rec.Model,
		"metadata":   rec.Metadata,
		"created_at": rec.CreatedAt,
	}

	if err := s.saveWithOutbox(ctx, rec, outboxPayload); err != nil {
		return nil, err
	}
	return rec, nil
}

func (s *Service) saveWithOutbox(ctx context.Context, rec *FeedbackRecord, payload map[string]interface{}) error {
	if s == nil || s.collection == nil || s.outbox == nil {
		return errors.New("feedback/outbox collection no disponible")
	}
	if rec == nil {
		return errors.New("feedback record inválido")
	}

	session, err := s.client.StartSession()
	if err == nil {
		defer session.EndSession(ctx)
		_, txnErr := session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
			res, insertErr := s.collection.InsertOne(sc, rec)
			if insertErr != nil {
				return nil, insertErr
			}
			if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
				rec.ID = oid.Hex()
			}
			event := outboxservice.BuildOutboxEvent(
				"feedback",
				rec.ID,
				"FeedbackSaved",
				payload,
				5,
			)
			_, outErr := outboxservice.InsertEvent(sc, s.outbox, event)
			if outErr != nil {
				return nil, outErr
			}
			return nil, nil
		})
		if txnErr == nil {
			return nil
		}
		s.logger.Warn("feedback transaction/outbox falló; fallback secuencial", slog.String("error", txnErr.Error()))
	}

	res, err := s.collection.InsertOne(ctx, rec)
	if err != nil {
		return err
	}
	if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
		rec.ID = oid.Hex()
	}
	event := outboxservice.BuildOutboxEvent(
		"feedback",
		rec.ID,
		"FeedbackSaved",
		payload,
		5,
	)
	if _, err := outboxservice.InsertEvent(ctx, s.outbox, event); err != nil {
		_, _ = s.collection.DeleteOne(ctx, bson.M{"_id": res.InsertedID})
		return err
	}
	return nil
}

func (s *Service) Summary(ctx context.Context, windowHours int) (FeedbackSummary, error) {
	if s == nil || s.collection == nil {
		return FeedbackSummary{}, errors.New("feedback service no disponible")
	}
	if windowHours <= 0 {
		windowHours = 24 * 7
	}
	since := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	cur, err := s.collection.Find(ctx,
		bson.M{"created_at": bson.M{"$gte": since}},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}),
	)
	if err != nil {
		return FeedbackSummary{}, err
	}
	defer cur.Close(ctx)

	var total int
	var sum float64
	type agg struct {
		Count int
		Sum   float64
	}
	modelAgg := make(map[string]agg)

	for cur.Next(ctx) {
		var rec FeedbackRecord
		if err := cur.Decode(&rec); err != nil {
			continue
		}
		total++
		sum += float64(rec.Rating)
		m := strings.TrimSpace(rec.Model)
		if m == "" {
			m = "unknown"
		}
		a := modelAgg[m]
		a.Count++
		a.Sum += float64(rec.Rating)
		modelAgg[m] = a
	}

	byModel := make([]ModelFeedbackSummary, 0, len(modelAgg))
	for model, a := range modelAgg {
		avg := 0.0
		if a.Count > 0 {
			avg = a.Sum / float64(a.Count)
		}
		byModel = append(byModel, ModelFeedbackSummary{
			Model:         model,
			Count:         a.Count,
			AverageRating: avg,
			Score:         normalizeRatingToScore(avg),
		})
	}
	sort.Slice(byModel, func(i, j int) bool {
		if byModel[i].Score == byModel[j].Score {
			return byModel[i].Count > byModel[j].Count
		}
		return byModel[i].Score > byModel[j].Score
	})

	avg := 0.0
	if total > 0 {
		avg = sum / float64(total)
	}

	return FeedbackSummary{
		WindowHours:   windowHours,
		TotalCount:    total,
		AverageRating: avg,
		ByModel:       byModel,
	}, nil
}

func (s *Service) GetModelFeedbackScore(ctx context.Context, model string) (float64, error) {
	if s == nil || s.collection == nil {
		return 0, errors.New("feedback service no disponible")
	}
	cleanModel := strings.TrimSpace(model)
	if cleanModel == "" {
		return 0.5, nil
	}

	cur, err := s.collection.Find(ctx,
		bson.M{"model": cleanModel},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(200),
	)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)

	count := 0
	sum := 0.0
	for cur.Next(ctx) {
		var rec FeedbackRecord
		if err := cur.Decode(&rec); err != nil {
			continue
		}
		count++
		sum += float64(rec.Rating)
	}
	if count == 0 {
		return 0.5, nil
	}
	avg := sum / float64(count)
	return normalizeRatingToScore(avg), nil
}

func normalizeRatingToScore(avg float64) float64 {
	if avg <= 1 {
		return 0
	}
	if avg >= 5 {
		return 1
	}
	return (avg - 1) / 4
}
