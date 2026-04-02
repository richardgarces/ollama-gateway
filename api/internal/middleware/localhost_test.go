package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalhostOnlyAllowsLoopback(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	h := LocalhostOnly(next)
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || !called {
		t.Fatalf("expected request allowed")
	}
}

func TestLocalhostOnlyBlocksRemote(t *testing.T) {
	h := LocalhostOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.RemoteAddr = "8.8.8.8:443"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestIsLocalRequestUsesForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "8.8.8.8:443"
	req.Header.Set("X-Forwarded-For", "127.0.0.1, 8.8.8.8")

	if !isLocalRequest(req) {
		t.Fatalf("expected local request from forwarded-for")
	}
}

func TestFirstForwardedFor(t *testing.T) {
	if got := firstForwardedFor(" 10.0.0.1 , 8.8.8.8"); got != "10.0.0.1" {
		t.Fatalf("unexpected forwarded-for first host: %q", got)
	}
}
