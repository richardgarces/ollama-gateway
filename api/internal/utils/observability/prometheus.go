package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollama_gateway_http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "path"},
	)

	httpRequestErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollama_gateway_http_errors_total",
			Help: "Total HTTP errors (status>=400)",
		},
		[]string{"method", "path"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollama_gateway_http_request_duration_seconds",
			Help:    "HTTP request durations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	rateLimitDecisions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollama_gateway_rate_limit_decisions_total",
			Help: "Rate limit decisions by user and endpoint",
		},
		[]string{"user_id", "endpoint", "action"},
	)
)

func init() {
	prometheus.MustRegister(httpRequests, httpRequestErrors, httpRequestDuration, rateLimitDecisions)
}

// ObservePrometheus records metrics into Prometheus collectors.
func ObservePrometheus(method, path string, status int, duration time.Duration) {
	httpRequests.WithLabelValues(method, path).Inc()
	if status >= 400 {
		httpRequestErrors.WithLabelValues(method, path).Inc()
	}
	httpRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

func ObserveRateLimit(userID, endpoint, action string) {
	if userID == "" {
		userID = "anonymous"
	}
	rateLimitDecisions.WithLabelValues(userID, endpoint, action).Inc()
}
