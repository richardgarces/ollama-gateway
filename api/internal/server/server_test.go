package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ollama-gateway/internal/config"
	eventservice "ollama-gateway/internal/function/events"
	"ollama-gateway/pkg/cache"
)

type mockIndexer struct {
	stopped bool
}

func (m *mockIndexer) IndexRepo() error    { return nil }
func (m *mockIndexer) StartWatcher() error { return nil }
func (m *mockIndexer) StopWatcher()        { m.stopped = true }
func (m *mockIndexer) ClearState() error   { return nil }

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
		"POST /api/admin/config/reload",
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

func TestStartReturnsErrorWithInvalidPort(t *testing.T) {
	s := &Server{
		cfg:    &config.Config{Port: "invalid-port"},
		router: http.NewServeMux(),
	}
	if err := s.Start(); err == nil {
		t.Fatalf("expected start error with invalid port")
	}
}

func TestStartAndShutdownLifecycle(t *testing.T) {
	s := &Server{
		cfg:    &config.Config{Port: "0"},
		router: http.NewServeMux(),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for s.httpServer == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.httpServer == nil {
		t.Fatalf("server did not initialize httpServer in time")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("start returned unexpected error after shutdown: %v", err)
	}
}

func TestCloseNilServer(t *testing.T) {
	s := &Server{}
	if err := s.Close(); err != nil {
		t.Fatalf("expected nil close error for nil http server, got %v", err)
	}
}

func TestShutdownCallsIndexerAndEventHooks(t *testing.T) {
	idx := &mockIndexer{}
	eventCtx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	bus := eventservice.NewBus(eventCtx, eventservice.Options{BufferSize: 4, Workers: 1}, nil)

	cancelCalled := false
	s := &Server{
		indexer:  idx,
		eventBus: bus,
		eventCancel: func() {
			cancelCalled = true
		},
	}

	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
	if !idx.stopped {
		t.Fatalf("expected indexer StopWatcher to be called")
	}
	if !cancelCalled {
		t.Fatalf("expected event cancel hook to be called")
	}
}

func TestThresholdOrFallback(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		fallback int
		want     int
	}{
		{name: "value positive", value: 5, fallback: 2, want: 5},
		{name: "fallback positive", value: 0, fallback: 4, want: 4},
		{name: "defaults to three", value: 0, fallback: 0, want: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := thresholdOrFallback(tc.value, tc.fallback); got != tc.want {
				t.Fatalf("thresholdOrFallback(%d,%d)=%d, want %d", tc.value, tc.fallback, got, tc.want)
			}
		})
	}
}
