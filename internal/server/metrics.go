package server

import (
	"context"
	"log/slog"
	"net/http"

	kratosmetrics "github.com/go-kratos/kratos/contrib/otel/v3/metrics"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// NewMeterProvider creates an OTel MeterProvider backed by a Prometheus exporter.
//
// The returned http.Handler serves the /metrics endpoint in Prometheus exposition
// format. The caller must call Shutdown on the returned provider before the
// process exits.
func NewMeterProvider(serviceName string) (*sdkmetric.MeterProvider, http.Handler, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, nil, err
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithView(
			kratosmetrics.DefaultSecondsHistogramView(kratosmetrics.DefaultServerSecondsHistogramName),
		),
	)
	otel.SetMeterProvider(provider)

	slog.Info("meter provider created, Prometheus exporter active")
	return provider, promhttp.Handler(), nil
}

// newServerMetricsMiddleware creates a Kratos server-side metrics middleware
// that records request counts and latency histograms via the global MeterProvider.
func newServerMetricsMiddleware(serviceName string) middleware.Middleware {
	meter := otel.GetMeterProvider().Meter(serviceName)

	requests, err := kratosmetrics.DefaultRequestsCounter(meter, kratosmetrics.DefaultServerRequestsCounterName)
	if err != nil {
		slog.Warn("failed to create requests counter, metrics disabled", "error", err)
		return func(handler middleware.Handler) middleware.Handler { return handler }
	}

	seconds, err := kratosmetrics.DefaultSecondsHistogram(meter, kratosmetrics.DefaultServerSecondsHistogramName)
	if err != nil {
		slog.Warn("failed to create seconds histogram, metrics disabled", "error", err)
		return func(handler middleware.Handler) middleware.Handler { return handler }
	}

	return kratosmetrics.Server(
		kratosmetrics.WithRequests(requests),
		kratosmetrics.WithSeconds(seconds),
	)
}

// Shutdown gracefully shuts down the MeterProvider, flushing any remaining metrics.
func ShutdownMeterProvider(ctx context.Context, mp *sdkmetric.MeterProvider) error {
	return mp.Shutdown(ctx)
}
