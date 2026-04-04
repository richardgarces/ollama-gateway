package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	eventservice "ollama-gateway/internal/function/events"
	"ollama-gateway/internal/function/pooling"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusProcessed  = "processed"
	StatusDead       = "dead"
)

type OutboxEvent struct {
	ID            primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	AggregateType string                 `bson:"aggregate_type" json:"aggregate_type"`
	AggregateID   string                 `bson:"aggregate_id" json:"aggregate_id"`
	EventType     string                 `bson:"event_type" json:"event_type"`
	Payload       map[string]interface{} `bson:"payload" json:"payload"`
	Status        string                 `bson:"status" json:"status"`
	Attempts      int                    `bson:"attempts" json:"attempts"`
	MaxAttempts   int                    `bson:"max_attempts" json:"max_attempts"`
	NextAttemptAt time.Time              `bson:"next_attempt_at" json:"next_attempt_at"`
	LastError     string                 `bson:"last_error,omitempty" json:"last_error,omitempty"`
	CreatedAt     time.Time              `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time              `bson:"updated_at" json:"updated_at"`
	ProcessedAt   *time.Time             `bson:"processed_at,omitempty" json:"processed_at,omitempty"`
}

type ForwardedEvent struct {
	Name    string                 `json:"name"`
	Payload map[string]interface{} `json:"payload"`
}

func (e ForwardedEvent) EventName() string { return e.Name }

type Service struct {
	client      *mongo.Client
	collection  *mongo.Collection
	publisher   eventservice.Publisher
	logger      *slog.Logger
	interval    time.Duration
	batchSize   int
	maxAttempts int
	backoff     time.Duration

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewService(mongoURI string, publisher eventservice.Publisher, interval time.Duration, batchSize, maxAttempts int, backoff time.Duration, logger *slog.Logger) (*Service, error) {
	return NewServiceWithPool(mongoURI, 0, 0, 5, publisher, interval, batchSize, maxAttempts, backoff, logger)
}

func NewServiceWithPool(mongoURI string, maxOpen, maxIdle, timeoutSeconds int, publisher eventservice.Publisher, interval time.Duration, batchSize, maxAttempts int, backoff time.Duration, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(mongoURI) == "" {
		return nil, errors.New("mongo uri requerido")
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}
	if batchSize <= 0 {
		batchSize = 25
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if backoff <= 0 {
		backoff = 5 * time.Second
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

	collection := client.Database("ollama_gateway").Collection("outbox_events")
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "next_attempt_at", Value: 1}}})
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: -1}}})
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "aggregate_type", Value: 1}, {Key: "aggregate_id", Value: 1}}})

	return &Service{
		client:      client,
		collection:  collection,
		publisher:   publisher,
		logger:      logger,
		interval:    interval,
		batchSize:   batchSize,
		maxAttempts: maxAttempts,
		backoff:     backoff,
	}, nil
}

func (s *Service) Start(parent context.Context) {
	if s == nil || s.collection == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.wg.Add(1)
	go s.worker(ctx)
}

func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if s.client != nil {
		return s.client.Disconnect(ctx)
	}
	return nil
}

func (s *Service) RetryDead(ctx context.Context, id string, allDead bool) (int64, error) {
	if s == nil || s.collection == nil {
		return 0, errors.New("outbox service no disponible")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()
	if allDead {
		res, err := s.collection.UpdateMany(ctx,
			bson.M{"status": StatusDead},
			bson.M{"$set": bson.M{"status": StatusPending, "next_attempt_at": now, "updated_at": now, "last_error": ""}},
		)
		if err != nil {
			return 0, err
		}
		return res.ModifiedCount, nil
	}

	objID, err := primitive.ObjectIDFromHex(strings.TrimSpace(id))
	if err != nil {
		return 0, errors.New("id inválido")
	}
	res, err := s.collection.UpdateOne(ctx,
		bson.M{"_id": objID, "status": StatusDead},
		bson.M{"$set": bson.M{"status": StatusPending, "next_attempt_at": now, "updated_at": now, "last_error": ""}},
	)
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

func (s *Service) worker(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processBatch(ctx)
		}
	}
}

func (s *Service) processBatch(ctx context.Context) {
	for i := 0; i < s.batchSize; i++ {
		ev, ok, err := s.claimNext(ctx)
		if err != nil {
			s.logger.Warn("outbox claim falló", slog.String("error", err.Error()))
			return
		}
		if !ok {
			return
		}
		if err := s.publish(ctx, ev); err != nil {
			_ = s.markFailure(ctx, ev, err)
			continue
		}
		_ = s.markProcessed(ctx, ev)
	}
}

func (s *Service) claimNext(ctx context.Context) (OutboxEvent, bool, error) {
	now := time.Now().UTC()
	filter := bson.M{"status": StatusPending, "next_attempt_at": bson.M{"$lte": now}}
	update := bson.M{"$set": bson.M{"status": StatusProcessing, "updated_at": now}}
	opts := options.FindOneAndUpdate().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetReturnDocument(options.After)
	var out OutboxEvent
	err := s.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&out)
	if err == mongo.ErrNoDocuments {
		return OutboxEvent{}, false, nil
	}
	if err != nil {
		return OutboxEvent{}, false, err
	}
	return out, true, nil
}

func (s *Service) publish(ctx context.Context, ev OutboxEvent) error {
	if s.publisher == nil {
		return errors.New("publisher no disponible")
	}
	if strings.TrimSpace(ev.EventType) == "" {
		return errors.New("event_type vacío")
	}
	return s.publisher.Publish(ctx, ForwardedEvent{Name: ev.EventType, Payload: ev.Payload})
}

func (s *Service) markProcessed(ctx context.Context, ev OutboxEvent) error {
	now := time.Now().UTC()
	_, err := s.collection.UpdateOne(ctx,
		bson.M{"_id": ev.ID},
		bson.M{"$set": bson.M{"status": StatusProcessed, "processed_at": now, "updated_at": now, "last_error": ""}},
	)
	return err
}

func (s *Service) markFailure(ctx context.Context, ev OutboxEvent, publishErr error) error {
	attempts := ev.Attempts + 1
	now := time.Now().UTC()
	status := StatusPending
	nextAttemptAt := now.Add(s.backoff * time.Duration(attempts))
	if attempts >= ev.MaxAttempts {
		status = StatusDead
		nextAttemptAt = now
	}
	_, err := s.collection.UpdateOne(ctx,
		bson.M{"_id": ev.ID},
		bson.M{"$set": bson.M{
			"status":          status,
			"attempts":        attempts,
			"next_attempt_at": nextAttemptAt,
			"updated_at":      now,
			"last_error":      truncateErr(publishErr),
		}},
	)
	if err == nil && status == StatusDead {
		s.logger.Warn("outbox event movido a dead-letter", slog.String("id", ev.ID.Hex()), slog.String("event_type", ev.EventType))
	}
	return err
}

func truncateErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) <= 500 {
		return msg
	}
	return msg[:500]
}

func BuildOutboxEvent(aggregateType, aggregateID, eventType string, payload map[string]interface{}, maxAttempts int) OutboxEvent {
	now := time.Now().UTC()
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	return OutboxEvent{
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       payload,
		Status:        StatusPending,
		Attempts:      0,
		MaxAttempts:   maxAttempts,
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func InsertEvent(ctx context.Context, c *mongo.Collection, ev OutboxEvent) (primitive.ObjectID, error) {
	if c == nil {
		return primitive.NilObjectID, errors.New("outbox collection no disponible")
	}
	res, err := c.InsertOne(ctx, ev)
	if err != nil {
		return primitive.NilObjectID, err
	}
	id, ok := res.InsertedID.(primitive.ObjectID)
	if !ok {
		return primitive.NilObjectID, fmt.Errorf("inserted id inválido")
	}
	return id, nil
}
