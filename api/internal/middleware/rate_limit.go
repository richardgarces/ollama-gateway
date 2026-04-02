package middleware

import (
	"net"
	"net/http"
	"strings"

	"ollama-gateway/internal/observability"
	"ollama-gateway/pkg/httputil"
)

func RateLimit(limiter *observability.RateLimiter, excludedPaths ...string) func(http.Handler) http.Handler {
	excluded := make(map[string]struct{}, len(excludedPaths))
	for _, path := range excludedPaths {
		excluded[path] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := excluded[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}

			clientIP := clientIPFromRequest(r)
			if !limiter.Allow(clientIP) {
				w.Header().Set("Retry-After", "60")
				httputil.WriteError(w, http.StatusTooManyRequests, "rate limit excedido")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func clientIPFromRequest(r *http.Request) string {
	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}

	if r.RemoteAddr == "" {
		return "unknown"
	}

	return r.RemoteAddr
}
