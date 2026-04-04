package runtimeconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ollama-gateway/internal/config"
)

func TestReloadUpdatesSharedConfig(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	path := filepath.Join(t.TempDir(), "runtime-config.json")
	t.Setenv("CONFIG_FILE", path)

	if err := os.WriteFile(path, []byte(`{"PORT":"8089","RATE_LIMIT_RPM":70,"HTTP_TIMEOUT_SECONDS":31}`), 0644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := config.LoadWithError()
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}
	svc := NewService(cfg)

	if err := os.WriteFile(path, []byte(`{"PORT":"8090","RATE_LIMIT_RPM":99,"HTTP_TIMEOUT_SECONDS":44}`), 0644); err != nil {
		t.Fatalf("update config file: %v", err)
	}

	result, err := svc.Reload()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.Port != "8090" || result.Port != "8090" {
		t.Fatalf("expected port reload to 8090, got cfg=%s result=%s", cfg.Port, result.Port)
	}
	if cfg.RateLimitRPM != 99 {
		t.Fatalf("expected rate limit reload to 99, got %d", cfg.RateLimitRPM)
	}
	if cfg.HTTPTimeoutSeconds != 44 {
		t.Fatalf("expected timeout reload to 44, got %d", cfg.HTTPTimeoutSeconds)
	}
}
