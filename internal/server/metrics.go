package server

import (
	"log/slog"

	kratosmetrics "github.com/go-kratos/kratos/contrib/otel/v3/metrics"
	"github.com/go-kratos/kratos/v3/middleware"
	"go.opentelemetry.io/otel"
)

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
