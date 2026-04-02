package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type QdrantService struct {
	baseURL     string
	client      *http.Client
	local       *diskVectorStore
	preferLocal bool
}

func NewQdrantService(baseURL string, repoRoot string, storePath string, preferLocal bool) *QdrantService {
	service := &QdrantService{
		baseURL:     baseURL,
		client:      &http.Client{Timeout: 20 * time.Second},
		preferLocal: preferLocal,
	}
	localStore, err := newDiskVectorStore(repoRoot, storePath)
	if err != nil {
		log.Printf("WARN: vector store local deshabilitado: %v", err)
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
	resp, err := q.client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		if q.local != nil {
			log.Printf("WARN: qdrant upsert falló, usando persistencia local: %v", err)
			return nil
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if q.local != nil {
			log.Printf("WARN: qdrant upsert devolvió status %d, usando persistencia local", resp.StatusCode)
			return nil
		}
		return fmt.Errorf("qdrant upsert failed status %d", resp.StatusCode)
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
	resp, err := q.client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		if q.local != nil {
			log.Printf("WARN: qdrant search falló, usando vector store local: %v", err)
			return q.local.Search(collection, vector, limit)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if q.local != nil {
			log.Printf("WARN: qdrant search devolvió status %d, usando vector store local", resp.StatusCode)
			return q.local.Search(collection, vector, limit)
		}
		return nil, fmt.Errorf("qdrant search failed status %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if q.local != nil {
			log.Printf("WARN: qdrant search no se pudo decodificar, usando vector store local: %v", err)
			return q.local.Search(collection, vector, limit)
		}
		return nil, err
	}
	return result, nil
}
