package infra

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
func NewTracerProvider(cfg *conf.Telemetry, info AppInfo) (*sdktrace.TracerProvider, func(), error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(info.Name),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler(cfg)),
	}

	if cfg != nil && cfg.OtlpEndpoint != "" {
		slog.Info("connecting to OTLP collector", "endpoint", cfg.OtlpEndpoint)

		conn, err := grpc.NewClient(cfg.OtlpEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("dial otlp collector: %w", err)
		}

		exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			return nil, nil, fmt.Errorf("create otlp exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
		slog.Info("OTLP exporter created, traces will be exported")
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Warn(err.Error())
	}))

	cleanup := func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			slog.Error("shutdown tracer provider", "error", err)
		}
	}
	return tp, cleanup, nil
}

func sampler(cfg *conf.Telemetry) sdktrace.Sampler {
	if cfg != nil && cfg.SamplingRate > 0 && cfg.SamplingRate < 1.0 {
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SamplingRate))
	}
	return sdktrace.AlwaysSample()
}
