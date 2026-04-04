package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"ollama-gateway/internal/function/pooling"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Migration struct {
	Version string
	Name    string
	Up      func(context.Context, *mongo.Database) error
	Down    func(context.Context, *mongo.Database) error
}

type AppliedMigration struct {
	Version   string    `bson:"version" json:"version"`
	Name      string    `bson:"name" json:"name"`
	AppliedAt time.Time `bson:"applied_at" json:"applied_at"`
}

type Runner struct {
	client      *mongo.Client
	db          *mongo.Database
	migrations  []Migration
	logger      *slog.Logger
	owner       string
	lockTTL     time.Duration
	migrationsC *mongo.Collection
	locksC      *mongo.Collection
}

func NewRunner(mongoURI string, logger *slog.Logger, lockTTL time.Duration) (*Runner, error) {
	return NewRunnerWithPool(mongoURI, 0, 0, 5, logger, lockTTL)
}

func NewRunnerWithPool(mongoURI string, maxOpen, maxIdle, timeoutSeconds int, logger *slog.Logger, lockTTL time.Duration) (*Runner, error) {
	if strings.TrimSpace(mongoURI) == "" {
		return nil, errors.New("mongo uri requerido")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if lockTTL <= 0 {
		lockTTL = 30 * time.Second
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

	db := client.Database("ollama_gateway")
	migrationsC := db.Collection("schema_migrations")
	locksC := db.Collection("migration_locks")

	_, _ = migrationsC.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "version", Value: 1}}, Options: options.Index().SetUnique(true)})
	_, _ = migrationsC.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "applied_at", Value: -1}}})
	_, _ = locksC.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "_id", Value: 1}}, Options: options.Index().SetUnique(true)})
	_, _ = locksC.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "expires_at", Value: 1}}})

	r := &Runner{
		client:      client,
		db:          db,
		migrations:  defaultMigrations(),
		logger:      logger,
		owner:       primitive.NewObjectID().Hex(),
		lockTTL:     lockTTL,
		migrationsC: migrationsC,
		locksC:      locksC,
	}
	return r, nil
}

func (r *Runner) Close(ctx context.Context) error {
	if r == nil || r.client == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return r.client.Disconnect(ctx)
}

func (r *Runner) List(ctx context.Context) ([]AppliedMigration, error) {
	if r == nil {
		return nil, errors.New("runner no disponible")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cur, err := r.migrationsC.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "version", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	out := make([]AppliedMigration, 0)
	for cur.Next(ctx) {
		var m AppliedMigration
		if err := cur.Decode(&m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *Runner) ApplyAll(ctx context.Context) error {
	return r.withLock(ctx, func(ctx context.Context) error {
		applied, err := r.appliedSet(ctx)
		if err != nil {
			return err
		}
		for _, m := range r.migrations {
			if applied[m.Version] {
				continue
			}
			if err := m.Up(ctx, r.db); err != nil {
				return fmt.Errorf("migration up %s failed: %w", m.Version, err)
			}
			if err := r.recordApplied(ctx, m); err != nil {
				return err
			}
			r.logger.Info("migration aplicada", slog.String("version", m.Version), slog.String("name", m.Name))
		}
		return nil
	})
}

func (r *Runner) RevertLast(ctx context.Context) error {
	return r.withLock(ctx, func(ctx context.Context) error {
		last, err := r.lastApplied(ctx)
		if err != nil {
			return err
		}
		if last == nil {
			return nil
		}
		mig, ok := r.findByVersion(last.Version)
		if !ok {
			return fmt.Errorf("no migration registered for version %s", last.Version)
		}
		if mig.Down == nil {
			return fmt.Errorf("migration %s has no down", mig.Version)
		}
		if err := mig.Down(ctx, r.db); err != nil {
			return fmt.Errorf("migration down %s failed: %w", mig.Version, err)
		}
		if _, err := r.migrationsC.DeleteOne(ctx, bson.M{"version": mig.Version}); err != nil {
			return err
		}
		r.logger.Info("migration revertida", slog.String("version", mig.Version), slog.String("name", mig.Name))
		return nil
	})
}

func (r *Runner) withLock(ctx context.Context, fn func(context.Context) error) error {
	if r == nil {
		return errors.New("runner no disponible")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := r.acquireLock(ctx); err != nil {
		return err
	}
	defer r.releaseLock(context.Background())
	return fn(ctx)
}

func (r *Runner) acquireLock(ctx context.Context) error {
	now := time.Now().UTC()
	expires := now.Add(r.lockTTL)
	filter := bson.M{
		"_id": "schema_migrations_lock",
		"$or": []bson.M{
			{"expires_at": bson.M{"$lte": now}},
			{"owner": r.owner},
			{"owner": bson.M{"$exists": false}},
		},
	}
	update := bson.M{
		"$set": bson.M{
			"owner":      r.owner,
			"expires_at": expires,
			"updated_at": now,
		},
		"$setOnInsert": bson.M{"created_at": now},
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var out bson.M
	err := r.locksC.FindOneAndUpdate(ctx, filter, update, opts).Decode(&out)
	if err == nil {
		return nil
	}
	if err != mongo.ErrNoDocuments {
		return err
	}
	return errors.New("no se pudo adquirir lock de migraciones")
}

func (r *Runner) releaseLock(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	_, _ = r.locksC.UpdateOne(
		ctx,
		bson.M{"_id": "schema_migrations_lock", "owner": r.owner},
		bson.M{"$set": bson.M{"expires_at": time.Now().UTC().Add(-1 * time.Second)}},
	)
}

func (r *Runner) appliedSet(ctx context.Context) (map[string]bool, error) {
	applied, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(applied))
	for _, m := range applied {
		out[m.Version] = true
	}
	return out, nil
}

func (r *Runner) recordApplied(ctx context.Context, m Migration) error {
	_, err := r.migrationsC.InsertOne(ctx, AppliedMigration{Version: m.Version, Name: m.Name, AppliedAt: time.Now().UTC()})
	return err
}

func (r *Runner) lastApplied(ctx context.Context) (*AppliedMigration, error) {
	var out AppliedMigration
	err := r.migrationsC.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.D{{Key: "version", Value: -1}})).Decode(&out)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *Runner) findByVersion(v string) (Migration, bool) {
	for _, m := range r.migrations {
		if m.Version == v {
			return m, true
		}
	}
	return Migration{}, false
}

func defaultMigrations() []Migration {
	m := []Migration{
		{
			Version: "2026040401",
			Name:    "create_outbox_events_indexes",
			Up: func(ctx context.Context, db *mongo.Database) error {
				c := db.Collection("outbox_events")
				_, _ = c.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "next_attempt_at", Value: 1}}})
				_, _ = c.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: -1}}})
				_, _ = c.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "aggregate_type", Value: 1}, {Key: "aggregate_id", Value: 1}}})
				return nil
			},
			Down: func(ctx context.Context, db *mongo.Database) error {
				c := db.Collection("outbox_events")
				_, _ = c.Indexes().DropOne(ctx, "status_1_next_attempt_at_1")
				_, _ = c.Indexes().DropOne(ctx, "created_at_-1")
				_, _ = c.Indexes().DropOne(ctx, "aggregate_type_1_aggregate_id_1")
				return nil
			},
		},
		{
			Version: "2026040402",
			Name:    "create_schema_and_lock_indexes",
			Up: func(ctx context.Context, db *mongo.Database) error {
				migrationsC := db.Collection("schema_migrations")
				locksC := db.Collection("migration_locks")
				_, _ = migrationsC.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "version", Value: 1}}, Options: options.Index().SetUnique(true)})
				_, _ = migrationsC.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "applied_at", Value: -1}}})
				_, _ = locksC.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "_id", Value: 1}}, Options: options.Index().SetUnique(true)})
				_, _ = locksC.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "expires_at", Value: 1}}})
				return nil
			},
			Down: func(ctx context.Context, db *mongo.Database) error {
				migrationsC := db.Collection("schema_migrations")
				locksC := db.Collection("migration_locks")
				_, _ = migrationsC.Indexes().DropOne(ctx, "version_1")
				_, _ = migrationsC.Indexes().DropOne(ctx, "applied_at_-1")
				_, _ = locksC.Indexes().DropOne(ctx, "_id_1")
				_, _ = locksC.Indexes().DropOne(ctx, "expires_at_1")
				return nil
			},
		},
	}
	sort.Slice(m, func(i, j int) bool { return m[i].Version < m[j].Version })
	return m
}
