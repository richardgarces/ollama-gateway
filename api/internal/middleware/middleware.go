package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"ollama-gateway/internal/utils/observability"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func Logging(next http.Handler) http.Handler {
	return LoggingWithStream(nil)(next)
}

func LoggingWithStream(stream *observability.LogStream) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rec, r)

			latency := time.Since(start)
			requestID := RequestIDFromContext(r.Context())
			traceID := observability.TraceIDFromContext(r.Context())
			tenantID := TenantFromContext(r.Context())
			slog.Info("http request",
				slog.String("request_id", requestID),
				slog.String("trace_id", traceID),
				slog.String("tenant_id", tenantID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.statusCode),
				slog.Duration("latency", latency),
			)

			if stream != nil {
				stream.Publish(observability.LogEvent{
					Timestamp: time.Now().UTC(),
					Level:     "info",
					Message:   "http request",
					Fields: map[string]interface{}{
						"request_id": requestID,
						"trace_id":   traceID,
						"tenant_id":  tenantID,
						"method":     r.Method,
						"path":       r.URL.Path,
						"status":     rec.statusCode,
						"latency_ms": latency.Milliseconds(),
					},
				})
			}
		})
	}
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
