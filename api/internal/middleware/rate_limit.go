package middleware

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/utils/observability"
	"ollama-gateway/pkg/httputil"
)

func RateLimit(limiter *observability.RateLimiter, cfg *config.Config, excludedPaths ...string) func(http.Handler) http.Handler {
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

			endpoint := strings.ToUpper(r.Method) + " " + r.URL.Path
			userID := UserIDFromContext(r.Context())
			clientIP := clientIPFromRequest(r)

			checks := buildRateChecks(cfg, clientIP, userID, endpoint)
			if len(checks) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			sort.Slice(checks, func(i, j int) bool {
				return checks[i].limit < checks[j].limit
			})

			for _, check := range checks {
				decision := limiter.Check(check.key, check.limit, false)
				if !decision.Allowed {
					setRateLimitHeaders(w, decision)
					observability.ObserveRateLimit(userID, endpoint, "rejected")
					httputil.WriteError(w, http.StatusTooManyRequests, "rate limit excedido")
					return
				}
			}

			minDecision := observability.RateLimitDecision{Allowed: true, Limit: checks[0].limit, Remaining: checks[0].limit}
			for _, check := range checks {
				decision := limiter.Check(check.key, check.limit, true)
				if decision.Limit < minDecision.Limit {
					minDecision = decision
				}
			}

			setRateLimitHeaders(w, minDecision)
			observability.ObserveRateLimit(userID, endpoint, "allowed")

			next.ServeHTTP(w, r)
		})
	}
}

type rateCheck struct {
	key   string
	limit int
}

func buildRateChecks(cfg *config.Config, clientIP, userID, endpoint string) []rateCheck {
	checks := make([]rateCheck, 0, 3)
	if cfg != nil && cfg.RateLimitRPM > 0 {
		checks = append(checks, rateCheck{key: "global:" + clientIP, limit: cfg.RateLimitRPM})
	}
	if cfg != nil && cfg.RateLimitUserRPM > 0 && userID != "" {
		checks = append(checks, rateCheck{key: "user:" + userID, limit: cfg.RateLimitUserRPM})
	}
	if cfg != nil && cfg.RateLimitEndpoints != nil {
		if endpointLimit, ok := cfg.RateLimitEndpoints[endpoint]; ok && endpointLimit > 0 {
			checks = append(checks, rateCheck{key: fmt.Sprintf("endpoint:%s:%s", clientIP, endpoint), limit: endpointLimit})
		}
	}
	return checks
}

func setRateLimitHeaders(w http.ResponseWriter, decision observability.RateLimitDecision) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(decision.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(decision.Remaining))
	if !decision.ResetAt.IsZero() {
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(decision.ResetAt.Unix(), 10))
	}
	if decision.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(decision.RetryAfter))
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
