package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"ollama-gateway/internal/config"
)

func TestReadiness_HealthyWithOptionalFailures(t *testing.T) {
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpSrv.Close()

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer tcpListener.Close()

	cfg := &config.Config{
		OllamaURL:            httpSrv.URL,
		QdrantURL:            httpSrv.URL,
		MongoURI:             "mongodb://" + tcpListener.Addr().String(),
		RedisURL:             "redis://" + tcpListener.Addr().String() + "/0",
		HealthCheckTimeoutMS: 500,
		HealthExtraChecksJSON: `[
			{"name":"optional-down","type":"http","target":"http://127.0.0.1:1","required":false}
		]`,
	}

	svc := NewService(cfg)
	resp := svc.Readiness(context.Background())

	if resp.Status != "healthy" {
		t.Fatalf("expected healthy, got %s", resp.Status)
	}

	for _, dep := range []string{"ollama", "qdrant", "mongo", "redis"} {
		if got, ok := resp.Dependencies[dep]; !ok || got.Status != "healthy" {
			t.Fatalf("expected dependency %s healthy, got %+v", dep, got)
		}
	}

	if optional, ok := resp.Dependencies["optional-down"]; !ok {
		t.Fatalf("expected optional check in response")
	} else if optional.Status != "unhealthy" {
		t.Fatalf("expected optional check unhealthy, got %+v", optional)
	}
}

func TestReadiness_DegradedWhenRequiredDependencyFails(t *testing.T) {
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpSrv.Close()

	cfg := &config.Config{
		OllamaURL:            httpSrv.URL,
		QdrantURL:            "http://127.0.0.1:1",
		HealthCheckTimeoutMS: 500,
	}

	svc := NewService(cfg)
	resp := svc.Readiness(context.Background())

	if resp.Status != "degraded" {
		t.Fatalf("expected degraded status, got %s (%+v)", resp.Status, resp.Dependencies)
	}
}

func TestReadiness_ExtraChecksSupportsTCP(t *testing.T) {
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer tcpListener.Close()

	cfg := &config.Config{
		HealthCheckTimeoutMS: 500,
		HealthExtraChecksJSON: fmt.Sprintf(`[
			{"name":"tcp-extra","type":"tcp","target":"%s","required":true}
		]`, tcpListener.Addr().String()),
	}

	svc := NewService(cfg)
	resp := svc.Readiness(context.Background())

	if resp.Status != "healthy" {
		t.Fatalf("expected healthy status, got %s (%+v)", resp.Status, resp.Dependencies)
	}
	if dep, ok := resp.Dependencies["tcp-extra"]; !ok || dep.Status != "healthy" {
		t.Fatalf("expected tcp-extra healthy, got %+v", dep)
	}
}

func TestNewServiceStrict_InvalidJSON(t *testing.T) {
	cfg := &config.Config{
		HealthExtraChecksJSON: `[{"bad json`,
	}
	_, err := NewServiceStrict(cfg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNewServiceStrict_EmptyName(t *testing.T) {
	cfg := &config.Config{
		HealthExtraChecksJSON: `[{"name":"","type":"http","target":"http://x","required":false}]`,
	}
	_, err := NewServiceStrict(cfg)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestNewServiceStrict_DuplicateName(t *testing.T) {
	cfg := &config.Config{
		HealthExtraChecksJSON: `[
			{"name":"dup","type":"http","target":"http://a","required":false},
			{"name":"dup","type":"http","target":"http://b","required":false}
		]`,
	}
	_, err := NewServiceStrict(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestNewServiceStrict_UnsupportedType(t *testing.T) {
	cfg := &config.Config{
		HealthExtraChecksJSON: `[{"name":"x","type":"grpc","target":"localhost:50051","required":false}]`,
	}
	_, err := NewServiceStrict(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestCheckBackend_Found(t *testing.T) {
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpSrv.Close()

	cfg := &config.Config{
		OllamaURL:            httpSrv.URL,
		HealthCheckTimeoutMS: 500,
	}
	svc := NewService(cfg)

	status, found := svc.CheckBackend(context.Background(), "ollama")
	if !found {
		t.Fatal("expected ollama backend to be found")
	}
	if status.Status != "healthy" {
		t.Fatalf("expected healthy, got %s", status.Status)
	}
}

func TestCheckBackend_NotFound(t *testing.T) {
	cfg := &config.Config{HealthCheckTimeoutMS: 200}
	svc := NewService(cfg)

	_, found := svc.CheckBackend(context.Background(), "nonexistent")
	if found {
		t.Fatal("expected backend not found")
	}
}

func TestRegisteredBackends(t *testing.T) {
	cfg := &config.Config{
		OllamaURL:            "http://localhost:11434",
		QdrantURL:            "http://localhost:6333",
		HealthCheckTimeoutMS: 200,
	}
	svc := NewService(cfg)
	backends := svc.RegisteredBackends()

	if len(backends) < 2 {
		t.Fatalf("expected at least 2 backends, got %d: %v", len(backends), backends)
	}
}
