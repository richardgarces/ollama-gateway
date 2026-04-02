package config

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestParseRepoRootsAndLoad(t *testing.T) {
	t.Setenv("REPO_ROOTS", " ./a, ./b ,, ")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	t.Setenv("RATE_LIMIT_ENDPOINTS", `{"GET /x":10,"POST /y":0}`)
	cfg := Load()
	if len(cfg.RepoRoots) < 2 {
		t.Fatalf("expected repo roots parsed, got %+v", cfg.RepoRoots)
	}
	if cfg.RateLimitEndpoints["GET /x"] != 10 {
		t.Fatalf("expected endpoint limit parsed")
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Setenv("INT_OK", "10")
	t.Setenv("INT_BAD", "x")
	if v := getEnvAsInt("INT_OK", 1); v != 10 {
		t.Fatalf("expected int parse")
	}
	if v := getEnvAsInt("INT_BAD", 1); v != 1 {
		t.Fatalf("expected fallback on bad int")
	}

	t.Setenv("BOOL_OK", "true")
	t.Setenv("BOOL_BAD", "oops")
	if !getEnvAsBool("BOOL_OK", false) {
		t.Fatalf("expected bool true")
	}
	if v := getEnvAsBool("BOOL_BAD", true); !v {
		t.Fatalf("expected fallback on bad bool")
	}
}

func TestLoadJWTSecretPaths(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("ab", 32))
	decoded := loadJWTSecret()
	expected, _ := hex.DecodeString(strings.Repeat("ab", 32))
	if string(decoded) != string(expected) {
		t.Fatalf("expected decoded hex jwt secret")
	}

	t.Setenv("JWT_SECRET", "plain-secret")
	plain := loadJWTSecret()
	if string(plain) != "plain-secret" {
		t.Fatalf("expected plain jwt secret")
	}

	_ = os.Unsetenv("JWT_SECRET")
	randSecret := loadJWTSecret()
	if len(randSecret) != 32 {
		t.Fatalf("expected random 32-byte jwt secret")
	}
}
