package infra

import (
	"context"
	"log/slog"
	"net/http"

	kratosmetrics "github.com/go-kratos/kratos/contrib/otel/v3/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// NewMeterProvider creates an OTel MeterProvider backed by a Prometheus exporter.
func NewMeterProvider() (http.Handler, func(), error) {
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

	cleanup := func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			slog.Error("shutdown meter provider", "error", err)
		}
	}

	slog.Info("meter provider created, Prometheus exporter active")
	return promhttp.Handler(), cleanup, nil
}
