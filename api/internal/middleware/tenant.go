package middleware

import (
	"context"
	"net/http"
	"regexp"
	"strings"
)

type tenantContextKey string

const tenantKey tenantContextKey = "tenant-id"
const defaultTenant = "default"

var tenantIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

func Tenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := extractTenantID(r)
		ctx := context.WithValue(r.Context(), tenantKey, tenant)
		w.Header().Set("X-Tenant-ID", tenant)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TenantFromContext(ctx context.Context) string {
	if ctx == nil {
		return defaultTenant
	}
	v, _ := ctx.Value(tenantKey).(string)
	v = strings.TrimSpace(v)
	if v == "" {
		return defaultTenant
	}
	return v
}

func extractTenantID(r *http.Request) string {
	if r == nil {
		return defaultTenant
	}
	candidates := []string{
		r.Header.Get("X-Tenant-ID"),
		r.Header.Get("X-Tenant"),
		r.URL.Query().Get("tenant"),
	}
	for _, c := range candidates {
		trimmed := strings.TrimSpace(c)
		if trimmed == "" {
			continue
		}
		if tenantIDPattern.MatchString(trimmed) {
			return trimmed
		}
	}
	return defaultTenant
}
