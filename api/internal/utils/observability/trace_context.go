package observability

import (
	"context"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.TraceID().String()
}
