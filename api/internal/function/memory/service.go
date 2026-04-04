package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/internal/function/pooling"
	"ollama-gateway/pkg/reposcope"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	defaultMemoryTTLHours        = 24 * 30
	defaultMemoryPruneMaxEntries = 1500
)

type Service struct {
	client          *mongo.Client
	collection      *mongo.Collection
	ollama          domain.OllamaClient
	qdrant          domain.VectorStore
	logger          *slog.Logger
	repoRoot        string
	ttl             time.Duration
	pruneMaxEntries int
}

type SaveContextInput struct {
	Summary  string                 `json:"summary"`
	Detail   string                 `json:"detail"`
	Priority int                    `json:"priority"`
	Tags     []string               `json:"tags"`
	Source   string                 `json:"source"`
	Metadata map[string]interface{} `json:"metadata"`
	TTLHours int                    `json:"ttl_hours"`
}

type MemoryEvent struct {
	ID           string                 `json:"id" bson:"_id"`
	RepoRoot     string                 `json:"repo_root" bson:"repo_root"`
	Summary      string                 `json:"summary" bson:"summary"`
	Detail       string                 `json:"detail,omitempty" bson:"detail,omitempty"`
	Priority     int                    `json:"priority" bson:"priority"`
	Tags         []string               `json:"tags,omitempty" bson:"tags,omitempty"`
	Source       string                 `json:"source,omitempty" bson:"source,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty" bson:"metadata,omitempty"`
	CreatedAt    time.Time              `json:"created_at" bson:"created_at"`
	ExpiresAt    time.Time              `json:"expires_at" bson:"expires_at"`
	LastAccessAt time.Time              `json:"last_access_at" bson:"last_access_at"`
	Score        float64                `json:"score,omitempty" bson:"-"`
}

func NewService(
	mongoURI string,
	ollama domain.OllamaClient,
	qdrant domain.VectorStore,
	repoRoot string,
	ttlHours int,
	pruneMaxEntries int,
	logger *slog.Logger,
) (*Service, error) {
	return NewServiceWithPool(
		mongoURI,
		0,
		0,
		5,
		ollama,
		qdrant,
		repoRoot,
		ttlHours,
		pruneMaxEntries,
		logger,
	)
}

func NewServiceWithPool(
	mongoURI string,
	maxOpen, maxIdle, timeoutSeconds int,
	ollama domain.OllamaClient,
	qdrant domain.VectorStore,
	repoRoot string,
	ttlHours int,
	pruneMaxEntries int,
	logger *slog.Logger,
) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(mongoURI) == "" {
		return nil, errors.New("mongo uri requerido")
	}
	if ollama == nil {
		return nil, errors.New("ollama client requerido")
	}
	if qdrant == nil {
		return nil, errors.New("qdrant vector store requerido")
	}
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot = "."
	}
	if ttlHours <= 0 {
		ttlHours = defaultMemoryTTLHours
	}
	if pruneMaxEntries <= 0 {
		pruneMaxEntries = defaultMemoryPruneMaxEntries
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

	collection := client.Database("ollama_gateway").Collection("semantic_memory")
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "repo_root", Value: 1}, {Key: "created_at", Value: -1}}})
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "repo_root", Value: 1}, {Key: "priority", Value: -1}}})
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)})

	return &Service{
		client:          client,
		collection:      collection,
		ollama:          ollama,
		qdrant:          qdrant,
		logger:          logger,
		repoRoot:        repoRoot,
		ttl:             time.Duration(ttlHours) * time.Hour,
		pruneMaxEntries: pruneMaxEntries,
	}, nil
}

func (s *Service) SaveContext(ctx context.Context, in SaveContextInput) (*MemoryEvent, error) {
	if s == nil {
		return nil, errors.New("memory service no disponible")
	}
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		return nil, errors.New("summary requerido")
	}

	now := time.Now().UTC()
	priority := clampPriority(in.Priority)
	ttl := s.ttl
	if in.TTLHours > 0 {
		ttl = time.Duration(in.TTLHours) * time.Hour
	}

	event := &MemoryEvent{
		ID:           randomID(),
		RepoRoot:     s.repoRoot,
		Summary:      summary,
		Detail:       strings.TrimSpace(in.Detail),
		Priority:     priority,
		Tags:         normalizeTags(in.Tags),
		Source:       strings.TrimSpace(in.Source),
		Metadata:     in.Metadata,
		CreatedAt:    now,
		LastAccessAt: now,
		ExpiresAt:    now.Add(ttl),
	}

	if _, err := s.collection.InsertOne(ctx, event); err != nil {
		return nil, err
	}

	embInput := event.Summary
	if event.Detail != "" {
		embInput = embInput + "\n" + event.Detail
	}
	emb, err := s.ollama.GetEmbedding("nomic-embed-text", embInput)
	if err != nil {
		s.logger.Warn("embedding de memoria falló", slog.String("error", err.Error()))
	} else {
		payload := map[string]interface{}{
			"id":         event.ID,
			"repo_root":  event.RepoRoot,
			"summary":    event.Summary,
			"priority":   event.Priority,
			"expires_at": event.ExpiresAt.Unix(),
			"created_at": event.CreatedAt.Unix(),
			"source":     event.Source,
		}
		if err := s.qdrant.UpsertPoint(s.vectorCollection(), event.ID, emb, payload); err != nil {
			s.logger.Warn("upsert qdrant de memoria falló", slog.String("error", err.Error()))
		}
	}

	if err := s.prune(ctx); err != nil {
		s.logger.Warn("pruning de memoria falló", slog.String("error", err.Error()))
	}
	return event, nil
}

func (s *Service) GetRelevantContext(ctx context.Context, query string, topK int) ([]MemoryEvent, error) {
	if s == nil {
		return nil, errors.New("memory service no disponible")
	}
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return nil, errors.New("query requerida")
	}
	if topK <= 0 {
		topK = 5
	}

	emb, err := s.ollama.GetEmbedding("nomic-embed-text", cleanQuery)
	if err != nil {
		return nil, err
	}
	searchRes, err := s.qdrant.Search(s.vectorCollection(), emb, topK*4)
	if err != nil {
		return s.fallbackRecent(ctx, topK)
	}

	items, _ := searchRes["result"].([]interface{})
	ids := make([]string, 0, len(items))
	scores := make(map[string]float64)
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		payload, _ := m["payload"].(map[string]interface{})
		id := strings.TrimSpace(toString(payload["id"]))
		if id == "" {
			id = strings.TrimSpace(toString(m["id"]))
		}
		if id == "" {
			continue
		}
		ids = append(ids, id)
		scores[id] = toFloat(m["score"])
	}
	if len(ids) == 0 {
		return s.fallbackRecent(ctx, topK)
	}

	now := time.Now().UTC()
	cur, err := s.collection.Find(ctx, bson.M{
		"_id":        bson.M{"$in": ids},
		"repo_root":  s.repoRoot,
		"expires_at": bson.M{"$gt": now},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	out := make([]MemoryEvent, 0, topK)
	for cur.Next(ctx) {
		var e MemoryEvent
		if err := cur.Decode(&e); err != nil {
			continue
		}
		e.Score = scores[e.ID]
		out = append(out, e)
	}

	sort.Slice(out, func(i, j int) bool {
		left := out[i].Score + (float64(out[i].Priority) * 0.01)
		right := out[j].Score + (float64(out[j].Priority) * 0.01)
		if left == right {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return left > right
	})
	if len(out) > topK {
		out = out[:topK]
	}

	idsToTouch := make([]string, 0, len(out))
	for _, e := range out {
		idsToTouch = append(idsToTouch, e.ID)
	}
	if len(idsToTouch) > 0 {
		_, _ = s.collection.UpdateMany(ctx,
			bson.M{"_id": bson.M{"$in": idsToTouch}},
			bson.M{"$set": bson.M{"last_access_at": now}},
		)
	}

	return out, nil
}

func (s *Service) GetRelevantContextText(ctx context.Context, query string, topK int) (string, error) {
	items, err := s.GetRelevantContext(ctx, query, topK)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", nil
	}
	var b strings.Builder
	for _, item := range items {
		b.WriteString("- [priority ")
		b.WriteString(fmt.Sprintf("%d", item.Priority))
		b.WriteString("] ")
		b.WriteString(item.Summary)
		if item.Detail != "" {
			b.WriteString(" | detail: ")
			b.WriteString(item.Detail)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func (s *Service) fallbackRecent(ctx context.Context, topK int) ([]MemoryEvent, error) {
	now := time.Now().UTC()
	cur, err := s.collection.Find(ctx,
		bson.M{"repo_root": s.repoRoot, "expires_at": bson.M{"$gt": now}},
		options.Find().SetSort(bson.D{{Key: "priority", Value: -1}, {Key: "created_at", Value: -1}}).SetLimit(int64(topK)),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]MemoryEvent, 0, topK)
	for cur.Next(ctx) {
		var e MemoryEvent
		if err := cur.Decode(&e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *Service) prune(ctx context.Context) error {
	now := time.Now().UTC()
	_, _ = s.collection.DeleteMany(ctx, bson.M{"repo_root": s.repoRoot, "expires_at": bson.M{"$lte": now}})

	cur, err := s.collection.Find(ctx,
		bson.M{"repo_root": s.repoRoot, "expires_at": bson.M{"$gt": now}},
		options.Find().SetSort(bson.D{{Key: "priority", Value: -1}, {Key: "created_at", Value: -1}}).SetLimit(int64(s.pruneMaxEntries+1)),
	)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	keep := make([]string, 0, s.pruneMaxEntries)
	count := 0
	for cur.Next(ctx) {
		var e MemoryEvent
		if err := cur.Decode(&e); err != nil {
			continue
		}
		count++
		if len(keep) < s.pruneMaxEntries {
			keep = append(keep, e.ID)
		}
	}
	if count <= s.pruneMaxEntries {
		return nil
	}

	if len(keep) == 0 {
		_, err = s.collection.DeleteMany(ctx, bson.M{"repo_root": s.repoRoot})
		return err
	}
	_, err = s.collection.DeleteMany(ctx, bson.M{"repo_root": s.repoRoot, "_id": bson.M{"$nin": keep}})
	return err
}

func (s *Service) vectorCollection() string {
	return reposcope.CollectionName(s.repoRoot) + "_memory"
}

func clampPriority(v int) int {
	if v <= 0 {
		return 5
	}
	if v > 10 {
		return 10
	}
	return v
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{})
	for _, tag := range tags {
		clean := strings.ToLower(strings.TrimSpace(tag))
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func randomID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func toString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func toFloat(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	default:
		return 0
	}
}
