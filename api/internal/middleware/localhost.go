package middleware

import (
	"net"
	"net/http"
	"strings"

	"ollama-gateway/pkg/httputil"
)

func LocalhostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalRequest(r) {
			httputil.WriteError(w, http.StatusForbidden, "forbidden: localhost only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalRequest(r *http.Request) bool {
	if r == nil {
		return false
	}

	host := strings.TrimSpace(firstForwardedFor(r.Header.Get("X-Forwarded-For")))
	if host == "" {
		h, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			host = h
		} else {
			host = strings.TrimSpace(r.RemoteAddr)
		}
	}

	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func firstForwardedFor(v string) string {
	if v == "" {
		return ""
	}
	parts := strings.Split(v, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
