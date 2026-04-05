package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func Trace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prop := otel.GetTextMapPropagator()
		ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		tracer := otel.Tracer("ollama-gateway/http")
		spanName := r.Method + " " + r.URL.Path
		ctx, span := tracer.Start(ctx, spanName,
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.path", r.URL.Path),
				attribute.String("tenant.id", TenantFromContext(r.Context())),
				attribute.String("feature.name", featureFromPath(r.URL.Path)),
			),
		)
		defer span.End()

		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))

		span.SetAttributes(attribute.Int("http.status_code", rec.statusCode))
		if rec.statusCode >= 500 {
			span.SetStatus(codes.Error, http.StatusText(rec.statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		prop.Inject(ctx, propagation.HeaderCarrier(rec.Header()))
	})
}

func featureFromPath(path string) string {
	if path == "" || path == "/" {
		return "root"
	}
	if path == "/metrics" || path == "/metrics/prometheus" || path == "/metrics/value" {
		return "metrics"
	}
	trimmed := path
	if len(trimmed) > 0 && trimmed[0] == '/' {
		trimmed = trimmed[1:]
	}
	parts := splitPath(trimmed)
	if len(parts) == 0 {
		return "root"
	}
	if parts[0] == "api" {
		if len(parts) >= 3 && (parts[1] == "v1" || parts[1] == "v2") {
			return parts[2]
		}
		if len(parts) >= 2 {
			return parts[1]
		}
		return "api"
	}
	return parts[0]
}

func splitPath(path string) []string {
	out := make([]string, 0, 6)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				out = append(out, path[start:i])
			}
			start = i + 1
		}
	}
	return out
}
