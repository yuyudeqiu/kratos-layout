package main

import (
	"context"
	"flag"
	"os"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"

	zapLogger "github.com/go-kratos/kratos/contrib/log/zap/v2"
	"go.uber.org/zap"
	_ "go.uber.org/automaxprocs"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string
	// Version is the version of the compiled software.
	Version string
	// flagconf is the config flag.
	flagconf string

	id, _ = os.Hostname()
)

// loadConfig loads configuration from path.
func loadConfig(path string) (*conf.Bootstrap, func()) {
	c := config.New(
		config.WithSource(
			file.NewSource(path),
		),
	)
	cleanup := func() {
		_ = c.Close()
	}

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		cleanup()
		panic(err)
	}
	return &bc, cleanup
}

// initLogger initializes log.Logger with configuration and level overrides.
func initLogger(bc *conf.Bootstrap) log.Logger {
	var baseLogger log.Logger
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = os.Getenv("ENV")
	}

	logFormat := "console"
	logLevel := "info"
	if bc.Logger != nil {
		if bc.Logger.Format != "" {
			logFormat = bc.Logger.Format
		}
		if bc.Logger.Level != "" {
			logLevel = bc.Logger.Level
		}
	}

	if env == "prod" || env == "production" || logFormat == "json" {
		cfg := zap.NewProductionConfig()
		cfg.DisableCaller = true      // 禁用 Zap 自身的 caller 追踪，避免捕获到适配器内部的文件与行号
		cfg.EncoderConfig.TimeKey = "" // 禁用 Zap 自身的时间戳字段，避免与 Kratos 的 ts 重复
		z, _ := cfg.Build()
		baseLogger = zapLogger.NewLogger(z)
	} else {
		baseLogger = log.NewStdLogger(os.Stdout)
	}

	return log.With(log.NewFilter(baseLogger, log.FilterLevel(log.ParseLevel(logLevel))),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", id,
		"service.name", Name,
		"service.version", Version,
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)
}

// initTracerProvider sets up global OpenTelemetry tracer provider and returns a shutdown function.
func initTracerProvider(logger log.Logger) func() {
	traceEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if traceEndpoint == "" {
		traceEndpoint = "localhost:4318"
	}
	serviceName := Name
	if serviceName == "" {
		serviceName = "kratos-trace"
	}

	exporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(traceEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		panic(err)
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.ParentBased(tracesdk.TraceIDRatioBased(1.0))),
		tracesdk.WithBatcher(exporter),
		tracesdk.WithResource(resource.NewSchemaless(
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("exporter", "otlp"),
		)),
	)
	otel.SetTracerProvider(tp)

	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.NewHelper(logger).Errorf("failed to shutdown tracer provider: %v", err)
		}
	}
}

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func newApp(logger log.Logger, gs *grpc.Server, hs *http.Server) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(
			gs,
			hs,
		),
	)
}

func main() {
	flag.Parse()

	// 1. Load configuration
	bc, closeConfig := loadConfig(flagconf)
	defer closeConfig()

	// 2. Initialize structure logger
	logger := initLogger(bc)

	// 3. Initialize OpenTelemetry global tracer provider
	closeTracer := initTracerProvider(logger)
	defer closeTracer()

	// 4. Wire and run
	app, cleanup, err := wireApp(bc.Server, bc.Data, logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// start and wait for stop signal
	if err := app.Run(); err != nil {
		panic(err)
	}
}
