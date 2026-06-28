package infra

import (
	"net/http"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/google/wire"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ProviderSet is infrastructure providers shared across application layers.
var ProviderSet = wire.NewSet(
	ProvideServerConfig,
	ProvideDataConfig,
	ProvideLogConfig,
	ProvideTelemetryConfig,
	NewLogger,
	NewTracerProvider,
	NewMeterProvider,
	NewRuntime,
)

// AppInfo describes the running service.
type AppInfo struct {
	ID      string
	Name    string
	Version string
}

// Runtime keeps process-level infrastructure in the Wire graph.
type Runtime struct {
	TracerProvider *sdktrace.TracerProvider
	MetricsHandler http.Handler
}

func ProvideServerConfig(c *conf.Bootstrap) *conf.Server {
	return c.Server
}

func ProvideDataConfig(c *conf.Bootstrap) *conf.Data {
	return c.Data
}

func ProvideLogConfig(c *conf.Bootstrap) *conf.Log {
	return c.Log
}

func ProvideTelemetryConfig(c *conf.Bootstrap) *conf.Telemetry {
	return c.Telemetry
}

func NewRuntime(tp *sdktrace.TracerProvider, metricsHandler http.Handler) Runtime {
	return Runtime{
		TracerProvider: tp,
		MetricsHandler: metricsHandler,
	}
}
