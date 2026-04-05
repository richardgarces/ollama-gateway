package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ollama-gateway/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func InitTracing(cfg *config.Config) (func(context.Context) error, error) {
	if cfg == nil || !cfg.OTelEnabled {
		return func(context.Context) error { return nil }, nil
	}

	endpoint := strings.TrimSpace(cfg.OTelExporterOTLPEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT requerido cuando OTEL_ENABLED=true")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if cfg.OTelExporterInsecure {
		clientOpts = append(clientOpts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}

	samplePercent := cfg.OTelSamplePercent
	if samplePercent < 0 {
		samplePercent = 0
	}
	if samplePercent > 100 {
		samplePercent = 100
	}

	serviceName := strings.TrimSpace(cfg.OTelServiceName)
	if serviceName == "" {
		serviceName = "ollama-gateway"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(float64(samplePercent)/100.0))),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
