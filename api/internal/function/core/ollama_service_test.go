package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/pkg/cache"
)

func TestOllamaServiceGenerateAndStream(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			if strings.Contains(r.URL.RawQuery, "bad") {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				_, _ = w.Write([]byte(`{"response":"ok"}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	svc := &OllamaService{baseURL: ts.URL, client: ts.Client(), cache: cache.NewMemory(), cacheTTL: time.Second}
	out, err := svc.Generate("m", "hola")
	if err != nil || out != "ok" {
		t.Fatalf("Generate failed: out=%q err=%v", out, err)
	}

	chunks := ""
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("{\"response\":\"a\",\"done\":false}\n{\"response\":\"b\",\"done\":true}\n"))
	})
	err = svc.StreamGenerate("m", "hola", func(s string) error {
		chunks += s
		return nil
	})
	if err != nil || chunks != "ab" {
		t.Fatalf("StreamGenerate failed: chunks=%q err=%v", chunks, err)
	}
}

func TestOllamaServiceGetEmbeddingCacheAndErrors(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			calls++
			if calls == 2 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("boom"))
				return
			}
			_, _ = w.Write([]byte(`{"embedding":[0.1,0.2,0.3]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	mem := cache.NewMemory()
	svc := &OllamaService{baseURL: ts.URL, client: ts.Client(), cache: mem, cacheTTL: time.Minute}
	emb1, err := svc.GetEmbedding("nomic", "hello")
	if err != nil || len(emb1) != 3 {
		t.Fatalf("first GetEmbedding failed: %v", err)
	}
	// second call should be cache hit
	emb2, err := svc.GetEmbedding("nomic", "hello")
	if err != nil || len(emb2) != 3 {
		t.Fatalf("cache GetEmbedding failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one remote call due to cache, got %d", calls)
	}

	// new text triggers remote, configured to fail on second remote call
	if _, err := svc.GetEmbedding("nomic", "other"); err == nil {
		t.Fatalf("expected embedding status error")
	}
}

func TestNewOllamaServiceUsesConfig(t *testing.T) {
	cfg := &config.Config{
		OllamaURL:                "http://example",
		HTTPTimeoutSeconds:       2,
		HTTPMaxRetries:           1,
		EmbeddingCacheTTLSeconds: 10,
		EmbeddingCacheMaxEntries: 100,
	}
	svc := NewOllamaService(cfg, nil, nil)
	if svc.baseURL != cfg.OllamaURL {
		t.Fatalf("unexpected baseURL: %s", svc.baseURL)
	}
	if svc.cache == nil || svc.client == nil {
		t.Fatalf("expected initialized cache and client")
	}
	if svc.maxCacheSize != 100 {
		t.Fatalf("unexpected maxCacheSize: %d", svc.maxCacheSize)
	}
	if fmt.Sprint(svc.cacheTTL) == "0s" {
		t.Fatalf("expected non-zero cacheTTL")
	}
}
