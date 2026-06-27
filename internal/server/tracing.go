package server

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// NewTracerProvider creates an OpenTelemetry TracerProvider.
//
// When cfg.OtlpEndpoint is set, spans are exported via OTLP/gRPC (e.g. to Grafana Tempo).
// When empty (local development), spans are still created so trace_id/span_id appear in
// logs, but they are not exported anywhere — no noise on stdout.
//
// The caller must call Shutdown on the returned provider before the process exits.
func NewTracerProvider(cfg *conf.Telemetry, serviceName string) (*sdktrace.TracerProvider, error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	}

	if cfg != nil && cfg.OtlpEndpoint != "" {
		exporter, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.OtlpEndpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("create otlp exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}
	// No exporter in dev mode: spans still created (trace_id/span_id in logs),
	// but not dumped anywhere.

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp, nil
}
