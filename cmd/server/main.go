package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/go-kratos/kratos-layout/internal/conf"
	"github.com/go-kratos/kratos-layout/internal/server"

	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/config"
	"github.com/go-kratos/kratos/v3/config/file"
	"github.com/go-kratos/kratos/v3/log"
	"github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/go-kratos/kratos/v3/transport/http"

	_ "go.uber.org/automaxprocs"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string = "kratos.layout"
	// Version is the version of the compiled software.
	Version string = "dev"
	// flagconf is the config flag.
	flagconf string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func newApp(logger *slog.Logger, gs *grpc.Server, hs *http.Server) *kratos.App {
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

// parseLevel parses a log level string, defaulting to info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// buildLogger creates a configured slog.Logger from Log config.
// It returns the logger, an optional cleanup function for file handles, and any error.
func buildLogger(cfg *conf.Log, id, name, version string) (*slog.Logger, func(), error) {
	var opts []log.Option

	// Format: default text
	if cfg != nil && strings.EqualFold(cfg.Format, "json") {
		opts = append(opts, log.WithFormat(log.FormatJSON))
	}

	// Level: default info
	level := slog.LevelInfo
	if cfg != nil && cfg.Level != "" {
		level = parseLevel(cfg.Level)
	}
	opts = append(opts, log.WithLevel(level))

	// AddSource
	addSource := false
	if cfg != nil {
		addSource = cfg.AddSource
	}
	opts = append(opts, log.WithAddSource(addSource))

	// Writer: stdout or file
	var cleanup func()
	writer := io.Writer(os.Stdout)
	if cfg != nil && cfg.OutputPath != "" {
		f, err := os.OpenFile(cfg.OutputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file %s: %w", cfg.OutputPath, err)
		}
		writer = f
		cleanup = func() { f.Close() }
	}
	opts = append(opts, log.WithWriter(writer))

	// Tracing extractor (preserves existing OpenTelemetry integration)
	opts = append(opts, log.WithExtractor(tracing.TraceAttrs))

	handler := log.NewHandler(opts...)
	logger := slog.New(handler).With(
		slog.String("service.id", id),
		slog.String("service.name", name),
		slog.String("service.version", version),
	)
	return logger, cleanup, nil
}

func main() {
	flag.Parse()

	// Phase 1: bootstrap logger for config loading
	bootstrapLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	log.SetDefault(bootstrapLogger)

	c := config.New(
		config.WithSource(
			file.NewSource(flagconf),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	// Phase 2: build configured logger from config
	logger, logCleanup, err := buildLogger(bc.Log, id, Name, Version)
	if err != nil {
		panic(err)
	}
	if logCleanup != nil {
		defer logCleanup()
	}
	log.SetDefault(logger)

	// Initialize OpenTelemetry TracerProvider.
	tp, err := server.NewTracerProvider(bc.Telemetry, Name)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Error("shutdown tracer provider", "error", err)
		}
	}()

	// Initialize OpenTelemetry MeterProvider (Prometheus exporter).
	mp, metricsHandler, err := server.NewMeterProvider(Name)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := mp.Shutdown(context.Background()); err != nil {
			log.Error("shutdown meter provider", "error", err)
		}
	}()

	app, cleanup, err := wireApp(bc.Server, bc.Data, logger, metricsHandler)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// start and wait for stop signal
	if err := app.Run(); err != nil {
		panic(err)
	}
}
