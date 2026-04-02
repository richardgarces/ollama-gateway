package config

import (
	"os"
	"testing"
)

func TestGetEnvAsInt(t *testing.T) {
	t.Setenv("X_INT", "42")
	if got := getEnvAsInt("X_INT", 1); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	t.Setenv("X_INT", "bad")
	if got := getEnvAsInt("X_INT", 7); got != 7 {
		t.Fatalf("expected fallback 7, got %d", got)
	}
}

func TestGetEnvAsBool(t *testing.T) {
	t.Setenv("X_BOOL", "true")
	if !getEnvAsBool("X_BOOL", false) {
		t.Fatalf("expected true")
	}
	t.Setenv("X_BOOL", "0")
	if getEnvAsBool("X_BOOL", true) {
		t.Fatalf("expected false")
	}
}

func TestGetEnvAsIntMap(t *testing.T) {
	t.Setenv("X_MAP", `{"GET /a":2,"POST /b":0}`)
	m := getEnvAsIntMap("X_MAP", map[string]int{"fallback": 1})
	if m["GET /a"] != 2 {
		t.Fatalf("expected endpoint value 2")
	}
	if _, ok := m["POST /b"]; ok {
		t.Fatalf("expected zero value key removed")
	}
}

func TestParseRepoRootsFallback(t *testing.T) {
	t.Setenv("REPO_ROOTS", "")
	t.Setenv("REPO_ROOT", ".")
	roots := parseRepoRoots()
	if len(roots) == 0 {
		t.Fatalf("expected fallback repo root")
	}
}

func TestLoadConfigCoreFields(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("OLLAMA_URL", "http://localhost:11434")
	t.Setenv("QDRANT_URL", "http://localhost:6333")
	t.Setenv("JWT_SECRET", "test-secret-with-32-chars-minimum")
	t.Setenv("REPO_ROOT", ".")
	t.Setenv("MONGO_URI", "mongodb://localhost:27017")

	cfg := Load()
	if cfg.Port != "9999" {
		t.Fatalf("expected port 9999, got %s", cfg.Port)
	}
	if cfg.OllamaURL == "" || cfg.QdrantURL == "" {
		t.Fatalf("expected non-empty service URLs")
	}
	if len(cfg.JWTSecret) == 0 {
		t.Fatalf("expected JWT secret loaded")
	}
}

func TestLoadJWTSecretGeneratedWhenMissing(t *testing.T) {
	_ = os.Unsetenv("JWT_SECRET")
	b := loadJWTSecret()
	if len(b) == 0 {
		t.Fatalf("expected generated secret")
	}
}
