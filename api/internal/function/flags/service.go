package flags

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/function/pooling"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	defaultTTL        = 30 * time.Second
	defaultTenant     = "default"
	globalTenant      = "*"
	flagsDatabaseName = "ollama_gateway"
	flagsCollection   = "feature_flags"
)

type Service struct {
	client     *mongo.Client
	collection *mongo.Collection
	logger     *slog.Logger
	ttl        time.Duration

	cacheMu sync.RWMutex
	cache   map[string]cacheEntry
}

type cacheEntry struct {
	flag      *Flag
	found     bool
	expiresAt time.Time
}

type Flag struct {
	Tenant            string     `json:"tenant" bson:"tenant"`
	Feature           string     `json:"feature" bson:"feature"`
	Enabled           bool       `json:"enabled" bson:"enabled"`
	RolloutPercentage int        `json:"rollout_percentage" bson:"rollout_percentage"`
	StartAt           *time.Time `json:"start_at,omitempty" bson:"start_at,omitempty"`
	EndAt             *time.Time `json:"end_at,omitempty" bson:"end_at,omitempty"`
	Description       string     `json:"description,omitempty" bson:"description,omitempty"`
	CreatedAt         time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" bson:"updated_at"`
}

type UpsertInput struct {
	Tenant            string     `json:"tenant"`
	Feature           string     `json:"feature"`
	Enabled           bool       `json:"enabled"`
	RolloutPercentage int        `json:"rollout_percentage"`
	StartAt           *time.Time `json:"start_at,omitempty"`
	EndAt             *time.Time `json:"end_at,omitempty"`
	Description       string     `json:"description,omitempty"`
}

func NewService(mongoURI string, logger *slog.Logger) (*Service, error) {
	return NewServiceWithPool(mongoURI, 0, 0, 5, defaultTTL, logger)
}

func NewServiceWithPool(mongoURI string, maxOpen, maxIdle, timeoutSeconds int, ttl time.Duration, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(mongoURI) == "" {
		return nil, errors.New("mongo uri requerido")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	client, err := pooling.ConnectMongo(mongoURI, maxOpen, maxIdle, timeoutSeconds)
	if err != nil {
		return nil, err
	}

	collection := client.Database(flagsDatabaseName).Collection(flagsCollection)
	_, _ = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "tenant", Value: 1}, {Key: "feature", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	return &Service{
		client:     client,
		collection: collection,
		logger:     logger,
		ttl:        ttl,
		cache:      make(map[string]cacheEntry),
	}, nil
}

func (s *Service) Disconnect(ctx context.Context) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Disconnect(ctx)
}

func (s *Service) IsEnabled(tenant, feature string) (bool, error) {
	return s.IsEnabledWithContext(context.Background(), tenant, feature)
}

func (s *Service) IsEnabledWithContext(ctx context.Context, tenant, feature string) (bool, error) {
	if s == nil || s.collection == nil {
		return false, errors.New("flags service no disponible")
	}
	nTenant, nFeature, err := normalizeTenantFeature(tenant, feature)
	if err != nil {
		return false, err
	}

	now := time.Now().UTC()
	if cached, ok := s.getCache(cacheKey(nTenant, nFeature), now); ok {
		if !cached.found || cached.flag == nil {
			return false, nil
		}
		return evaluateEnabled(*cached.flag, nTenant, now), nil
	}

	flag, found, err := s.findFlag(ctx, nTenant, nFeature)
	if err != nil {
		return false, err
	}
	s.setCache(cacheKey(nTenant, nFeature), cacheEntry{flag: flag, found: found, expiresAt: now.Add(s.ttl)})

	if !found || flag == nil {
		return false, nil
	}
	return evaluateEnabled(*flag, nTenant, now), nil
}

func (s *Service) Create(ctx context.Context, input UpsertInput) (*Flag, error) {
	flag, err := validateAndBuildFlag(input, time.Time{}, false)
	if err != nil {
		return nil, err
	}

	if _, err := s.collection.InsertOne(ctx, flag); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("flag ya existe para tenant+feature")
		}
		return nil, err
	}
	s.invalidate(flag.Tenant, flag.Feature)
	out := *flag
	return &out, nil
}

func (s *Service) List(ctx context.Context, tenant string) ([]Flag, error) {
	if s == nil || s.collection == nil {
		return nil, errors.New("flags service no disponible")
	}
	filter := bson.M{}
	if strings.TrimSpace(tenant) != "" {
		filter["tenant"] = strings.TrimSpace(strings.ToLower(tenant))
	}
	cur, err := s.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "tenant", Value: 1}, {Key: "feature", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	out := make([]Flag, 0, 16)
	for cur.Next(ctx) {
		var flag Flag
		if err := cur.Decode(&flag); err != nil {
			continue
		}
		out = append(out, flag)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, tenant, feature string) (*Flag, error) {
	if s == nil || s.collection == nil {
		return nil, errors.New("flags service no disponible")
	}
	nTenant, nFeature, err := normalizeTenantFeature(tenant, feature)
	if err != nil {
		return nil, err
	}

	var flag Flag
	if err := s.collection.FindOne(ctx, bson.M{"tenant": nTenant, "feature": nFeature}).Decode(&flag); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("flag no encontrado")
		}
		return nil, err
	}
	return &flag, nil
}

func (s *Service) Update(ctx context.Context, tenant, feature string, input UpsertInput) (*Flag, error) {
	if s == nil || s.collection == nil {
		return nil, errors.New("flags service no disponible")
	}
	nTenant, nFeature, err := normalizeTenantFeature(tenant, feature)
	if err != nil {
		return nil, err
	}

	current, err := s.Get(ctx, nTenant, nFeature)
	if err != nil {
		return nil, err
	}

	input.Tenant = nTenant
	input.Feature = nFeature
	flag, err := validateAndBuildFlag(input, current.CreatedAt, true)
	if err != nil {
		return nil, err
	}

	res := s.collection.FindOneAndReplace(ctx,
		bson.M{"tenant": nTenant, "feature": nFeature},
		flag,
		options.FindOneAndReplace().SetReturnDocument(options.After),
	)
	if err := res.Err(); err != nil {
		return nil, err
	}
	var updated Flag
	if err := res.Decode(&updated); err != nil {
		return nil, err
	}
	s.invalidate(nTenant, nFeature)
	return &updated, nil
}

func (s *Service) Delete(ctx context.Context, tenant, feature string) error {
	if s == nil || s.collection == nil {
		return errors.New("flags service no disponible")
	}
	nTenant, nFeature, err := normalizeTenantFeature(tenant, feature)
	if err != nil {
		return err
	}
	res, err := s.collection.DeleteOne(ctx, bson.M{"tenant": nTenant, "feature": nFeature})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return fmt.Errorf("flag no encontrado")
	}
	s.invalidate(nTenant, nFeature)
	return nil
}

func (s *Service) findFlag(ctx context.Context, tenant, feature string) (*Flag, bool, error) {
	var flag Flag
	err := s.collection.FindOne(ctx, bson.M{"tenant": tenant, "feature": feature}).Decode(&flag)
	if err == nil {
		return &flag, true, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, err
	}

	err = s.collection.FindOne(ctx, bson.M{"tenant": globalTenant, "feature": feature}).Decode(&flag)
	if err == nil {
		return &flag, true, nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, nil
	}
	return nil, false, err
}

func evaluateEnabled(flag Flag, tenant string, now time.Time) bool {
	if !flag.Enabled {
		return false
	}
	if flag.StartAt != nil && now.Before(flag.StartAt.UTC()) {
		return false
	}
	if flag.EndAt != nil && now.After(flag.EndAt.UTC()) {
		return false
	}
	r := flag.RolloutPercentage
	if r <= 0 {
		return false
	}
	if r >= 100 {
		return true
	}
	bucket := rolloutBucket(tenant, flag.Feature)
	return bucket < r
}

func rolloutBucket(tenant, feature string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(tenant)) + ":" + strings.ToLower(strings.TrimSpace(feature))))
	return int(h.Sum32() % 100)
}

func validateAndBuildFlag(input UpsertInput, createdAt time.Time, isUpdate bool) (*Flag, error) {
	tenant, feature, err := normalizeTenantFeature(input.Tenant, input.Feature)
	if err != nil {
		return nil, err
	}
	rollout := input.RolloutPercentage
	if rollout < 0 || rollout > 100 {
		return nil, fmt.Errorf("rollout_percentage debe estar entre 0 y 100")
	}
	if input.StartAt != nil && input.EndAt != nil && input.EndAt.UTC().Before(input.StartAt.UTC()) {
		return nil, fmt.Errorf("end_at no puede ser anterior a start_at")
	}

	now := time.Now().UTC()
	if !isUpdate || createdAt.IsZero() {
		createdAt = now
	}

	flag := &Flag{
		Tenant:            tenant,
		Feature:           feature,
		Enabled:           input.Enabled,
		RolloutPercentage: rollout,
		StartAt:           input.StartAt,
		EndAt:             input.EndAt,
		Description:       strings.TrimSpace(input.Description),
		CreatedAt:         createdAt,
		UpdatedAt:         now,
	}
	return flag, nil
}

func normalizeTenantFeature(tenant, feature string) (string, string, error) {
	t := strings.ToLower(strings.TrimSpace(tenant))
	if t == "" {
		t = defaultTenant
	}
	f := strings.ToLower(strings.TrimSpace(feature))
	if f == "" {
		return "", "", fmt.Errorf("feature requerido")
	}
	return t, f, nil
}

func cacheKey(tenant, feature string) string {
	return tenant + ":" + feature
}

func (s *Service) getCache(key string, now time.Time) (cacheEntry, bool) {
	s.cacheMu.RLock()
	entry, ok := s.cache[key]
	s.cacheMu.RUnlock()
	if !ok {
		return cacheEntry{}, false
	}
	if now.After(entry.expiresAt) {
		s.cacheMu.Lock()
		delete(s.cache, key)
		s.cacheMu.Unlock()
		return cacheEntry{}, false
	}
	return entry, true
}

func (s *Service) setCache(key string, entry cacheEntry) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache[key] = entry
}

func (s *Service) invalidate(tenant, feature string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	delete(s.cache, cacheKey(tenant, feature))
	if tenant != defaultTenant {
		delete(s.cache, cacheKey(defaultTenant, feature))
	}
}
