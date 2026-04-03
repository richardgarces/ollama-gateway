package service

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"ollama-gateway/internal/function/core/domain"
)

type vectorPoint struct {
	ID      string                 `json:"id"`
	Vector  []float64              `json:"vector"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

type diskVectorStore struct {
	path        string
	collections map[string]map[string]vectorPoint
	mu          sync.RWMutex
}

var _ domain.VectorStore = (*diskVectorStore)(nil)

func newDiskVectorStore(repoRoot, storePath string) (*diskVectorStore, error) {
	resolvedPath, err := resolvePathWithinRoot(repoRoot, storePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0755); err != nil {
		return nil, err
	}
	store := &diskVectorStore{
		path:        resolvedPath,
		collections: make(map[string]map[string]vectorPoint),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func resolvePathWithinRoot(repoRoot, targetPath string) (string, error) {
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	path := targetPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootAbs, path)
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rootPrefix := rootAbs + string(os.PathSeparator)
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootPrefix) {
		return "", fmt.Errorf("vector store path fuera de repo root: %s", pathAbs)
	}
	return pathAbs, nil
}

func (s *diskVectorStore) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var collections map[string]map[string]vectorPoint
	if err := json.Unmarshal(b, &collections); err != nil {
		return err
	}
	s.collections = collections
	return nil
}

func (s *diskVectorStore) persistLocked() error {
	data, err := json.MarshalIndent(s.collections, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func (s *diskVectorStore) UpsertPoint(collection, id string, vector []float64, payload map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.collections[collection]; !ok {
		s.collections[collection] = make(map[string]vectorPoint)
	}
	s.collections[collection][id] = vectorPoint{
		ID:      id,
		Vector:  append([]float64(nil), vector...),
		Payload: payload,
	}
	return s.persistLocked()
}

func (s *diskVectorStore) Search(collection string, vector []float64, limit int) (map[string]interface{}, error) {
	s.mu.RLock()
	points := s.collections[collection]
	s.mu.RUnlock()

	type scoredPoint struct {
		ID      string
		Score   float64
		Payload map[string]interface{}
	}

	results := make([]scoredPoint, 0, len(points))
	for _, point := range points {
		results = append(results, scoredPoint{
			ID:      point.ID,
			Score:   cosineSimilarity(vector, point.Vector),
			Payload: point.Payload,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	items := make([]map[string]interface{}, 0, limit)
	for _, item := range results[:limit] {
		items = append(items, map[string]interface{}{
			"id":      item.ID,
			"score":   item.Score,
			"payload": item.Payload,
		})
	}

	return map[string]interface{}{"result": items}, nil
}

func cosineSimilarity(left, right []float64) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	var dot float64
	var leftNorm float64
	var rightNorm float64
	for i := 0; i < limit; i++ {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}
