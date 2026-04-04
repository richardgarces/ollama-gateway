package transport

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ollama-gateway/internal/config"
	runtimeconfig "ollama-gateway/internal/function/runtime_config"
)

func TestReloadMethodValidation(t *testing.T) {
	h := NewHandler(nil)
	r := httptest.NewRequest(http.MethodGet, "/api/admin/config/reload", nil)
	w := httptest.NewRecorder()

	h.Reload(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestReloadServiceUnavailable(t *testing.T) {
	h := NewHandler(nil)
	r := httptest.NewRequest(http.MethodPost, "/api/admin/config/reload", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	h.Reload(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestReloadBadRequestOnInvalidConfig(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	path := filepath.Join(t.TempDir(), "runtime-config-invalid.json")
	t.Setenv("CONFIG_FILE", path)
	if err := os.WriteFile(path, []byte(`{"PORT":"8088"}`), 0644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	cfg, err := config.LoadWithError()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	h := NewHandler(runtimeconfig.NewService(cfg))

	if err := os.WriteFile(path, []byte(`{bad json`), 0644); err != nil {
		t.Fatalf("write invalid config file: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/admin/config/reload", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Reload(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestReloadSuccess(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	path := filepath.Join(t.TempDir(), "runtime-config.json")
	t.Setenv("CONFIG_FILE", path)
	if err := os.WriteFile(path, []byte(`{"PORT":"8088","RATE_LIMIT_RPM":75,"HTTP_TIMEOUT_SECONDS":22}`), 0644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := config.LoadWithError()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	svc := runtimeconfig.NewService(cfg)
	h := NewHandler(svc)

	if err := os.WriteFile(path, []byte(`{"PORT":"8087","RATE_LIMIT_RPM":80,"HTTP_TIMEOUT_SECONDS":19}`), 0644); err != nil {
		t.Fatalf("update config file: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/admin/config/reload", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Reload(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if cfg.Port != "8087" {
		t.Fatalf("expected cfg port updated, got %s", cfg.Port)
	}
}
