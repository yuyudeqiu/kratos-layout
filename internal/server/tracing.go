package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
		slog.Info("connecting to OTLP collector", "endpoint", cfg.OtlpEndpoint)

		// Use grpc.Dial with explicit insecure credentials — the deprecated
		// otlptracegrpc.WithInsecure() may be a no-op in newer OTel versions.
		conn, err := grpc.NewClient(cfg.OtlpEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, fmt.Errorf("dial otlp collector: %w", err)
		}

		exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			return nil, fmt.Errorf("create otlp exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
		slog.Info("OTLP exporter created, traces will be exported")
	}
	// No exporter in dev mode: spans still created (trace_id/span_id in logs),
	// but not dumped anywhere.

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Route OTel-internal errors (e.g. export failures) to slog.Warn so they
	// do not appear as misleading INFO messages.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Warn(err.Error())
	}))
	return tp, nil
}
