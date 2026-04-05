package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type AuthMiddleware struct {
	jwtSecret []byte
}

type AuditRecorder interface {
	RecordAuthzDenied(userID, role, requiredScope, method, path, reason, requestID string)
	RecordAuthnFailure(method, path, reason, requestID string)
}

type authContextKey string

const userIDContextKey authContextKey = "user-id"
const roleContextKey authContextKey = "role"
const scopesContextKey authContextKey = "scopes"

var roleScopes = map[string][]string{
	"admin":      {"*"},
	"maintainer": {"security:scan", "policy:enforce", "cicd:apply", "docs:apply", "patch:apply", "indexer:control", "audit:read", "jobs:manage", "jobs:read", "test:analyze"},
	"developer":  {"security:scan", "docs:apply", "jobs:read", "test:analyze"},
	"viewer":     {},
}

var auditRecorder AuditRecorder

func SetAuditRecorder(recorder AuditRecorder) {
	auditRecorder = recorder
}

func NewAuthMiddleware(jwtSecret []byte) *AuthMiddleware {
	return &AuthMiddleware{jwtSecret: jwtSecret}
}

func (m *AuthMiddleware) JWT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			if auditRecorder != nil {
				auditRecorder.RecordAuthnFailure(r.Method, r.URL.Path, "missing_or_invalid_authorization_header", RequestIDFromContext(r.Context()))
			}
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return m.jwtSecret, nil
		})

		if err != nil || !token.Valid {
			if auditRecorder != nil {
				auditRecorder.RecordAuthnFailure(r.Method, r.URL.Path, "invalid_token", RequestIDFromContext(r.Context()))
			}
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		userID := ""
		role := "viewer"
		scopes := []string{}
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if v, ok := claims["user"].(string); ok {
				userID = strings.TrimSpace(v)
			} else if v, ok := claims["sub"].(string); ok {
				userID = strings.TrimSpace(v)
			} else if v, ok := claims["user_id"].(string); ok {
				userID = strings.TrimSpace(v)
			} else if v, ok := claims["username"].(string); ok {
				userID = strings.TrimSpace(v)
			} else if v, ok := claims["email"].(string); ok {
				userID = strings.TrimSpace(v)
			}
			if v, ok := claims["role"].(string); ok {
				role = normalizeRole(v)
			}
			scopes = parseScopesFromClaims(claims)
		}

		if len(scopes) == 0 {
			scopes = defaultScopesForRole(role)
		}
		sort.Strings(scopes)

		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		ctx = context.WithValue(ctx, roleContextKey, role)
		ctx = context.WithValue(ctx, scopesContextKey, scopes)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequireScope(scope string) func(http.Handler) http.Handler {
	required := strings.TrimSpace(strings.ToLower(scope))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if required == "" {
				next.ServeHTTP(w, r)
				return
			}
			if !HasScope(r.Context(), required) {
				slog.Warn("authorization denied",
					slog.String("user_id", UserIDFromContext(r.Context())),
					slog.String("role", RoleFromContext(r.Context())),
					slog.String("required_scope", required),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("request_id", RequestIDFromContext(r.Context())),
				)
				if auditRecorder != nil {
					auditRecorder.RecordAuthzDenied(
						UserIDFromContext(r.Context()),
						RoleFromContext(r.Context()),
						required,
						r.Method,
						r.URL.Path,
						"scope_not_granted",
						RequestIDFromContext(r.Context()),
					)
				}
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func UserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(userIDContextKey).(string)
	return v
}

func RoleFromContext(ctx context.Context) string {
	if ctx == nil {
		return "viewer"
	}
	v, _ := ctx.Value(roleContextKey).(string)
	if strings.TrimSpace(v) == "" {
		return "viewer"
	}
	return normalizeRole(v)
}

func ScopesFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(scopesContextKey).([]string)
	out := make([]string, 0, len(v))
	for _, s := range v {
		ts := strings.TrimSpace(strings.ToLower(s))
		if ts != "" {
			out = append(out, ts)
		}
	}
	return out
}

func HasScope(ctx context.Context, scope string) bool {
	required := strings.TrimSpace(strings.ToLower(scope))
	if required == "" {
		return true
	}
	for _, s := range ScopesFromContext(ctx) {
		if s == "*" || s == required {
			return true
		}
	}
	return false
}

func normalizeRole(raw string) string {
	role := strings.TrimSpace(strings.ToLower(raw))
	if _, ok := roleScopes[role]; ok {
		return role
	}
	return "viewer"
}

func defaultScopesForRole(role string) []string {
	role = normalizeRole(role)
	v := roleScopes[role]
	out := make([]string, len(v))
	copy(out, v)
	return out
}

func parseScopesFromClaims(claims jwt.MapClaims) []string {
	out := make([]string, 0, 8)
	appendScope := func(v string) {
		for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' || r == ';' }) {
			ts := strings.TrimSpace(strings.ToLower(part))
			if ts != "" {
				out = append(out, ts)
			}
		}
	}
	if v, ok := claims["scope"].(string); ok {
		appendScope(v)
	}
	if v, ok := claims["scopes"].(string); ok {
		appendScope(v)
	}
	if arr, ok := claims["scopes"].([]interface{}); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				appendScope(s)
			}
		}
	}
	uniq := make(map[string]struct{}, len(out))
	dedup := make([]string, 0, len(out))
	for _, s := range out {
		if _, ok := uniq[s]; ok {
			continue
		}
		uniq[s] = struct{}{}
		dedup = append(dedup, s)
	}
	return dedup
}
