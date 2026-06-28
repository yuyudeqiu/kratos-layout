package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/go-kratos/kratos-layout/internal/conf"
	"github.com/go-kratos/kratos-layout/internal/infra"
	"github.com/go-kratos/kratos-layout/internal/migration"
	"github.com/go-kratos/kratos-layout/internal/server"

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
	// flagMigrations is the migrations directory flag.
	flagMigrations string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	flag.StringVar(&flagMigrations, "migrations", "../../migrations", "migration directory")
}

func main() {
	flag.Parse()

	bootstrapLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	log.SetDefault(bootstrapLogger)

	bc, err := loadBootstrap(flagconf)
	if err != nil {
		panic(err)
	}

	args := flag.Args()
	if isMigrateCommand(args) {
		if err := migration.Run(bc, flagMigrations, args[1:], bootstrapLogger); err != nil {
			bootstrapLogger.Error("run migration failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if err := bc.Validate(); err != nil {
		panic(err)
	}
	if err := bc.ValidateTelemetry(); err != nil {
		bootstrapLogger.Warn("telemetry config", "warning", err)
	}

	app, cleanup, err := wireApp(bc, infra.AppInfo{
		ID:      id,
		Name:    Name,
		Version: Version,
	})
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		panic(err)
	}
}

func newApp(logger *slog.Logger, _ infra.Runtime, _ server.Pprof, gs *grpc.Server, hs *http.Server) *kratos.App {
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

func isMigrateCommand(args []string) bool {
	return len(args) > 0 && strings.EqualFold(args[0], "migrate")
}

func loadBootstrap(path string) (*conf.Bootstrap, error) {
	c := config.New(
		config.WithSource(
			file.NewSource(path),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return &bc, nil
}
