package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ollama-gateway/internal/config"
	"ollama-gateway/pkg/cache"
)

func TestGetRouteDefinitions(t *testing.T) {
	routes := GetRouteDefinitions()
	if len(routes) == 0 {
		t.Fatalf("expected route definitions")
	}
	seen := map[string]bool{}
	for _, r := range routes {
		seen[r.Method+" "+r.Path] = true
	}
	must := []string{
		"GET /health",
		"POST /api/generate",
		"POST /api/commit/message",
		"GET /api-docs",
	}
	for _, m := range must {
		if !seen[m] {
			t.Fatalf("missing route definition %s", m)
		}
	}
}

func TestServerPublicRoutes(t *testing.T) {
	cfg := &config.Config{
		Port:         "8081",
		JWTSecret:    []byte("secret"),
		RepoRoot:     ".",
		RepoRoots:    []string{"."},
		QdrantURL:    "http://localhost:6333",
		OllamaURL:    "http://localhost:11434",
		MongoURI:     "mongodb://localhost:27017",
		CacheBackend: "memory",
	}
	s := New(cfg, cache.NewMemory())
	h := s.Handler()

	for _, path := range []string{"/health", "/health/liveness", "/health/readiness", "/metrics"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code == http.StatusNotFound {
			t.Fatalf("expected registered route for %s", path)
		}
	}
}
