package infra

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
	"github.com/go-kratos/kratos/v3/log"
)

// NewLogger creates a configured slog.Logger from Log config.
func NewLogger(cfg *conf.Log, info AppInfo) (*slog.Logger, func(), error) {
	var opts []log.Option

	if cfg != nil && strings.EqualFold(cfg.Format, "json") {
		opts = append(opts, log.WithFormat(log.FormatJSON))
	}

	level := slog.LevelInfo
	if cfg != nil && cfg.Level != "" {
		level = parseLevel(cfg.Level)
	}
	opts = append(opts, log.WithLevel(level))

	addSource := false
	if cfg != nil {
		addSource = cfg.AddSource
	}
	opts = append(opts, log.WithAddSource(addSource))

	var cleanup func()
	writer := io.Writer(os.Stdout)
	if cfg != nil && cfg.OutputPath != "" {
		f, err := os.OpenFile(cfg.OutputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file %s: %w", cfg.OutputPath, err)
		}
		writer = f
		cleanup = func() { _ = f.Close() }
	}
	opts = append(opts, log.WithWriter(writer))
	opts = append(opts, log.WithExtractor(tracing.TraceAttrs))

	handler := log.NewHandler(opts...)
	logger := slog.New(handler).With(
		slog.String("service.id", info.ID),
		slog.String("service.name", info.Name),
		slog.String("service.version", info.Version),
	)
	log.SetDefault(logger)
	return logger, cleanup, nil
}

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
