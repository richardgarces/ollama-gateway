package middleware

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/utils/observability"

	"github.com/golang-jwt/jwt/v5"
)

func TestRequestIDMiddleware(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Fatalf("expected request id in context")
		}
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatalf("expected X-Request-ID header")
	}
}

func TestCORS(t *testing.T) {
	h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/x", nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS")
	}
}

func TestLocalhostOnly(t *testing.T) {
	h := LocalhostOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected localhost request allowed")
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "8.8.8.8:9999"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected non-localhost request forbidden")
	}
}

func TestCompress(t *testing.T) {
	h := Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip response")
	}
	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip reader error: %v", err)
	}
	defer zr.Close()
	out, _ := io.ReadAll(zr)
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("unexpected compressed body: %s", string(out))
	}
}

func TestAuthMiddlewareJWT(t *testing.T) {
	mw := NewAuthMiddleware([]byte("secret"))
	h := mw.JWT(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserIDFromContext(r.Context()) != "admin" {
			t.Fatalf("expected user in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without header")
	}

	token := signedTokenForTest(t, []byte("secret"), map[string]interface{}{"user": "admin"})
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected authorized request")
	}
}

func TestAuthMiddlewareRoleAndScopes(t *testing.T) {
	mw := NewAuthMiddleware([]byte("secret"))
	h := mw.JWT(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RoleFromContext(r.Context()) != "maintainer" {
			t.Fatalf("expected maintainer role")
		}
		if !HasScope(r.Context(), "security:scan") {
			t.Fatalf("expected security:scan scope")
		}
		if HasScope(r.Context(), "unknown:scope") {
			t.Fatalf("did not expect unknown scope")
		}
		w.WriteHeader(http.StatusOK)
	}))

	token := signedTokenForTest(t, []byte("secret"), map[string]interface{}{
		"user":   "dev1",
		"role":   "maintainer",
		"scopes": []string{"security:scan", "docs:apply"},
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected authorized request")
	}
}

func TestRequireScope(t *testing.T) {
	mw := NewAuthMiddleware([]byte("secret"))
	h := mw.JWT(RequireScope("patch:apply")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	allowed := signedTokenForTest(t, []byte("secret"), map[string]interface{}{"user": "u1", "role": "admin"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/patch", nil)
	r.Header.Set("Authorization", "Bearer "+allowed)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected allowed request, got %d", w.Code)
	}

	denied := signedTokenForTest(t, []byte("secret"), map[string]interface{}{"user": "u2", "role": "viewer"})
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/patch", nil)
	r.Header.Set("Authorization", "Bearer "+denied)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden request, got %d", w.Code)
	}
}

func TestMetricsAndRateLimit(t *testing.T) {
	collector := observability.NewMetricsCollector()
	limiter := observability.NewRateLimiter(1, 100*time.Millisecond)
	cfg := &config.Config{RateLimitRPM: 1, RateLimitUserRPM: 0, RateLimitEndpoints: map[string]int{"GET /x": 1}}

	h := RequestID(RateLimit(limiter, cfg)(Metrics(collector)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))))

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "127.0.0.1:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected first request allowed")
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "127.0.0.1:1"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected rate limited second request")
	}

	s := collector.Snapshot()
	if s.TotalRequests == 0 {
		t.Fatalf("expected collected metrics")
	}

	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
}

func signedTokenForTest(t *testing.T, secret []byte, claims map[string]interface{}) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(claims))
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("token sign error: %v", err)
	}
	return signed
}
