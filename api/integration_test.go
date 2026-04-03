//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ollama-gateway/internal/config"
	coreservice "ollama-gateway/internal/function/core"
	indexerservice "ollama-gateway/internal/function/indexer"
	"ollama-gateway/internal/server"
	"ollama-gateway/pkg/cache"
	"ollama-gateway/pkg/reposcope"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupQdrant(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "qdrant/qdrant:latest",
		ExposedPorts: []string{"6333/tcp"},
		WaitingFor:   wait.ForHTTP("/").WithPort("6333/tcp").WithStartupTimeout(90 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("no se pudo iniciar contenedor qdrant: %v", err)
	}

	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("no se pudo obtener host de qdrant: %v", err)
	}
	port, err := container.MappedPort(ctx, "6333/tcp")
	if err != nil {
		t.Fatalf("no se pudo obtener puerto de qdrant: %v", err)
	}

	return fmt.Sprintf("http://%s:%s", host, port.Port())
}

func setupMongo(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "mongo:7",
		ExposedPorts: []string{"27017/tcp"},
		Env: map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": "admin",
			"MONGO_INITDB_ROOT_PASSWORD": "integration",
		},
		WaitingFor: wait.ForLog("Waiting for connections").WithStartupTimeout(90 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("no se pudo iniciar contenedor mongo: %v", err)
	}

	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("no se pudo obtener host de mongo: %v", err)
	}
	port, err := container.MappedPort(ctx, "27017/tcp")
	if err != nil {
		t.Fatalf("no se pudo obtener puerto de mongo: %v", err)
	}

	return fmt.Sprintf("mongodb://admin:integration@%s:%s/?authSource=admin", host, port.Port())
}

func TestFullRAGPipeline(t *testing.T) {
	t.Parallel()

	qdrantURL := setupQdrant(t)

	repoRoot := t.TempDir()
	fixture, err := os.ReadFile("testdata/sample.go")
	if err != nil {
		t.Fatalf("no se pudo leer fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "sample.go"), fixture, 0644); err != nil {
		t.Fatalf("no se pudo escribir fixture: %v", err)
	}

	fakeOllama := newFakeOllamaServer(t)

	cfg := &config.Config{
		OllamaURL:                fakeOllama.URL,
		RepoRoot:                 repoRoot,
		VectorStorePath:          filepath.Join(repoRoot, ".vector_store.json"),
		VectorStorePreferLocal:   false,
		IndexerStatePath:         filepath.Join(repoRoot, ".indexer_state.json"),
		HTTPTimeoutSeconds:       5,
		HTTPMaxRetries:           1,
		EmbeddingCacheTTLSeconds: 60,
		EmbeddingCacheMaxEntries: 100,
	}

	logger := slog.Default()
	cacheBackend := cache.NewMemory()
	ollama := coreservice.NewOllamaService(cfg, logger, cacheBackend)
	qdrant := coreservice.NewQdrantService(qdrantURL, repoRoot, cfg.VectorStorePath, false, 5, 1, logger)
	indexer, err := indexerservice.NewIndexerService([]string{repoRoot}, cfg.IndexerStatePath, ollama, qdrant, logger)
	if err != nil {
		t.Fatalf("NewIndexerService() error = %v", err)
	}
	router := coreservice.NewRouterService(cfg, ollama, logger)
	rag := coreservice.NewRAGService(ollama, router, qdrant, logger, cacheBackend, []string{repoRoot}, "en", 1800, 500)

	if err := indexer.IndexRepo(); err != nil {
		t.Fatalf("IndexRepo() error = %v", err)
	}

	emb, err := ollama.GetEmbedding("nomic-embed-text", "sum function")
	if err != nil {
		t.Fatalf("GetEmbedding() error = %v", err)
	}
	res, err := qdrant.Search(reposcope.CollectionName(repoRoot), emb, 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	items, ok := res["result"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("se esperaban resultados de búsqueda, got=%T len=%d", res["result"], len(items))
	}

	foundRelevant := false
	for _, raw := range items {
		row, _ := raw.(map[string]interface{})
		payload, _ := row["payload"].(map[string]interface{})
		code, _ := payload["code"].(string)
		if strings.Contains(code, "func Sum") {
			foundRelevant = true
			break
		}
	}
	if !foundRelevant {
		t.Fatalf("no se encontró código relevante en resultados")
	}

	out, err := rag.GenerateWithContext("How can I add two integers in Go?")
	if err != nil {
		t.Fatalf("GenerateWithContext() error = %v", err)
	}
	if out == "" {
		t.Fatalf("se esperaba respuesta no vacía de RAG")
	}
}

func TestOpenAIEndpointIntegration(t *testing.T) {
	t.Parallel()

	_ = setupMongo(t)

	repoRoot := t.TempDir()
	fakeOllama := newFakeOllamaServer(t)

	cfg := &config.Config{
		Port:                     "8081",
		OllamaURL:                fakeOllama.URL,
		QdrantURL:                "",
		JWTSecret:                []byte("01234567890123456789012345678901"),
		LogLevel:                 "info",
		LogFormat:                "json",
		RepoRoot:                 repoRoot,
		VectorStorePath:          filepath.Join(repoRoot, ".vector_store.json"),
		VectorStorePreferLocal:   true,
		IndexerStatePath:         filepath.Join(repoRoot, ".indexer_state.json"),
		RateLimitRPM:             1000,
		HTTPTimeoutSeconds:       5,
		HTTPMaxRetries:           1,
		EmbeddingCacheTTLSeconds: 60,
		EmbeddingCacheMaxEntries: 100,
	}

	srv := server.New(cfg, cache.NewMemory())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	body := map[string]interface{}{
		"model": "local-rag",
		"messages": []map[string]string{
			{"role": "user", "content": "hola"},
		},
	}
	payload, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+"/openai/v1/chat/completions", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("POST chat/completions error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status inesperado: %d body=%s", resp.StatusCode, string(b))
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
	if out["object"] != "chat.completion" {
		t.Fatalf("formato inesperado, object=%v", out["object"])
	}
	choices, ok := out["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		t.Fatalf("choices inválido en respuesta: %#v", out["choices"])
	}
}

func newFakeOllamaServer(t *testing.T) *httptest.Server {
	t.Helper()

	h := http.NewServeMux()
	h.HandleFunc("/api/embeddings", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		emb := deterministicEmbedding(req.Prompt)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"embedding": emb})
	})
	h.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"response": "integration-ok", "done": true})
	})

	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func deterministicEmbedding(text string) []float64 {
	if text == "" {
		return []float64{0.1, 0.2, 0.3, 0.4}
	}
	var total int
	for _, b := range []byte(text) {
		total += int(b)
	}
	base := float64(total%1000) / 1000.0
	return []float64{base + 0.1, base + 0.2, base + 0.3, base + 0.4}
}
