package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithDeprecationHeaders(t *testing.T) {
	h := WithDeprecationHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "/api/v2/generate", "2026-12-31")

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/generate", nil)
	h.ServeHTTP(w, r)

	if w.Header().Get("Deprecation") != "true" {
		t.Fatalf("expected Deprecation header")
	}
	if !strings.Contains(w.Header().Get("Link"), "/api/v2/generate") {
		t.Fatalf("expected Link successor-version header")
	}
	if w.Header().Get("X-API-Sunset-Date") != "2026-12-31" {
		t.Fatalf("expected X-API-Sunset-Date header")
	}
}

func TestWithJSONFieldAliases(t *testing.T) {
	h := WithJSONFieldAliases(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload["prompt"] != "hola" {
			t.Fatalf("expected translated prompt, got %v", payload["prompt"])
		}
		w.WriteHeader(http.StatusOK)
	}), map[string]string{"query": "prompt"})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v2/generate", strings.NewReader(`{"query":"hola"}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200")
	}
	if got := w.Header().Get("X-API-Translated-Fields"); got != "query->prompt" {
		t.Fatalf("expected translation header, got %q", got)
	}
}
