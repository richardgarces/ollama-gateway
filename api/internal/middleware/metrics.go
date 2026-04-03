package middleware

import (
	"net/http"
	"time"

	"ollama-gateway/internal/utils/observability"
)

func Metrics(collector *observability.MetricsCollector) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)
			dur := time.Since(start)
			collector.Observe(r.Method, r.URL.Path, rec.statusCode, dur)
			// also export to Prometheus if available
			observability.ObservePrometheus(r.Method, r.URL.Path, rec.statusCode, dur)
		})
	}
}
