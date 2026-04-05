package middleware

import (
	"context"
	"net/http"
	"strings"

	"ollama-gateway/pkg/httputil"
)

const defaultFlagTenant = "default"

type FeatureFlagEvaluator interface {
	IsEnabledWithContext(ctx context.Context, tenant, feature string) (bool, error)
}

// RequireFeatureFlag gates endpoint execution behind a feature flag check.
func RequireFeatureFlag(evaluator FeatureFlagEvaluator, feature string) func(http.Handler) http.Handler {
	feature = strings.TrimSpace(strings.ToLower(feature))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if evaluator == nil || feature == "" {
				next.ServeHTTP(w, r)
				return
			}

			tenant := tenantFromRequest(r)
			enabled, err := evaluator.IsEnabledWithContext(r.Context(), tenant, feature)
			if err != nil {
				httputil.WriteError(w, http.StatusServiceUnavailable, "feature flags no disponibles")
				return
			}
			if !enabled {
				httputil.WriteError(w, http.StatusForbidden, "feature deshabilitada: "+feature)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func tenantFromRequest(r *http.Request) string {
	if r == nil {
		return defaultFlagTenant
	}
	for _, v := range []string{
		r.Header.Get("X-Tenant-ID"),
		r.Header.Get("X-Tenant"),
		r.URL.Query().Get("tenant"),
	} {
		v = strings.TrimSpace(strings.ToLower(v))
		if v != "" {
			return v
		}
	}
	return defaultFlagTenant
}
