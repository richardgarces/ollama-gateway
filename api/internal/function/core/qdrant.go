package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/internal/function/resilience"
	"ollama-gateway/pkg/httpclient"
)

type QdrantService struct {
	baseURL     string
	client      *http.Client
	local       *diskVectorStore
	preferLocal bool
	logger      *slog.Logger
	breaker     *resilience.CircuitBreaker
}

var _ domain.VectorStore = (*QdrantService)(nil)

func NewQdrantService(baseURL string, repoRoot string, storePath string, preferLocal bool, timeoutSeconds int, maxRetries int, cbThreshold int, cbOpenTimeoutSeconds int, cbHalfOpenMaxSuccess int, poolMaxOpen int, poolMaxIdle int, poolTimeoutSeconds int, logger *slog.Logger) *QdrantService {
	if logger == nil {
		logger = slog.Default()
	}
	service := &QdrantService{
		baseURL: baseURL,
		client: httpclient.NewResilientClient(httpclient.Options{
			Timeout:             time.Duration(poolTimeoutSeconds) * time.Second,
			MaxRetries:          maxRetries,
			MaxConnsPerHost:     poolMaxOpen,
			MaxIdleConns:        poolMaxOpen,
			MaxIdleConnsPerHost: poolMaxIdle,
			IdleConnTimeout:     time.Duration(poolTimeoutSeconds) * time.Second,
			DialTimeout:         time.Duration(timeoutSeconds) * time.Second,
		}),
		preferLocal: preferLocal,
		logger:      logger,
		breaker: resilience.NewCircuitBreaker(resilience.Config{
			Name:               "qdrant",
			FailureThreshold:   cbThreshold,
			OpenTimeout:        time.Duration(cbOpenTimeoutSeconds) * time.Second,
			HalfOpenMaxSuccess: cbHalfOpenMaxSuccess,
		}),
	}
	localStore, err := newDiskVectorStore(repoRoot, storePath)
	if err != nil {
		service.logger.Warn("vector store local deshabilitado", slog.String("service", "qdrant"), slog.Any("error", err))
		return service
	}
	service.local = localStore
	return service
}

// UpsertPoint upserts a single point into a collection
func (q *QdrantService) UpsertPoint(collection string, id string, vector []float64, payload map[string]interface{}) error {
	if q.local != nil {
		if err := q.local.UpsertPoint(collection, id, vector, payload); err != nil {
			return err
		}
	}
	if q.preferLocal || q.baseURL == "" {
		return nil
	}
	url := fmt.Sprintf("%s/collections/%s/points?wait=true", q.baseURL, collection)
	body := map[string]interface{}{
		"points": []interface{}{
			map[string]interface{}{
				"id":      id,
				"vector":  vector,
				"payload": payload,
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	err = q.withBreaker(context.Background(), func(ctx context.Context) error {
		resp, postErr := q.client.Post(url, "application/json", bytes.NewBuffer(data))
		if postErr != nil {
			return postErr
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			// Collection missing — try to auto-create it.
			resp.Body.Close()
			if createErr := q.ensureCollection(collection, len(vector)); createErr != nil {
				return createErr
			}
			// Retry the upsert after collection creation.
			retryResp, retryErr := q.client.Post(url, "application/json", bytes.NewBuffer(data))
			if retryErr != nil {
				return retryErr
			}
			defer retryResp.Body.Close()
			if retryResp.StatusCode >= 300 {
				return fmt.Errorf("qdrant upsert retry failed status %d", retryResp.StatusCode)
			}
			return nil
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("qdrant upsert failed status %d", resp.StatusCode)
		}
		return nil
	})
	if err != nil {
		if q.local != nil {
			q.logger.Warn("qdrant upsert falló, usando persistencia local", slog.String("service", "qdrant"), slog.Any("error", err))
			return nil
		}
		return err
	}
	return nil
}

// Search performs a nearest-neighbors search in the given collection.
func (q *QdrantService) Search(collection string, vector []float64, limit int) (map[string]interface{}, error) {
	if q.preferLocal || q.baseURL == "" {
		if q.local == nil {
			return nil, fmt.Errorf("no vector store disponible")
		}
		return q.local.Search(collection, vector, limit)
	}
	url := fmt.Sprintf("%s/collections/%s/points/search", q.baseURL, collection)
	body := map[string]interface{}{
		"vector": vector,
		"limit":  limit,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	var notFound bool
	err = q.withBreaker(context.Background(), func(ctx context.Context) error {
		resp, postErr := q.client.Post(url, "application/json", bytes.NewBuffer(data))
		if postErr != nil {
			return postErr
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			// Collection does not exist — not a service failure.
			notFound = true
			return nil
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("qdrant search failed status %d", resp.StatusCode)
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil {
			return decodeErr
		}
		return nil
	})
	if notFound {
		q.logger.Debug("qdrant collection no existe, sin resultados", slog.String("collection", collection))
		return map[string]interface{}{"result": []interface{}{}}, nil
	}
	if err != nil {
		if q.local != nil {
			q.logger.Warn("qdrant search falló, usando vector store local", slog.String("service", "qdrant"), slog.Any("error", err))
			return q.local.Search(collection, vector, limit)
		}
		return nil, err
	}
	return result, nil
}

// ensureCollection creates a Qdrant collection if it does not already exist.
func (q *QdrantService) ensureCollection(collection string, vectorSize int) error {
	url := fmt.Sprintf("%s/collections/%s", q.baseURL, collection)
	body := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     vectorSize,
			"distance": "Cosine",
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("qdrant create collection %q failed status %d", collection, resp.StatusCode)
	}
	q.logger.Info("qdrant: colección creada automáticamente", slog.String("collection", collection), slog.Int("vector_size", vectorSize))
	return nil
}

func (q *QdrantService) CircuitBreakerState() resilience.Snapshot {
	if q == nil || q.breaker == nil {
		return resilience.Snapshot{Name: "qdrant", State: resilience.StateClosed}
	}
	return q.breaker.Snapshot()
}

func (q *QdrantService) withBreaker(ctx context.Context, op func(context.Context) error) error {
	if q == nil || q.breaker == nil {
		return op(ctx)
	}
	return q.breaker.Execute(ctx, op)
}
