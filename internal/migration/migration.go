package migration

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-kratos/kratos-layout/internal/conf"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
)

// Run executes a goose migration command against the configured database.
func Run(bc *conf.Bootstrap, migrationsDir string, args []string, logger *slog.Logger) error {
	command := "status"
	if len(args) > 0 {
		command = strings.ToLower(args[0])
		args = args[1:]
	}

	if err := validateDatabaseConfig(bc); err != nil {
		return err
	}
	if bc.Data.Database.Driver != "" && !strings.EqualFold(bc.Data.Database.Driver, "mysql") {
		return fmt.Errorf("unsupported database driver: %s", bc.Data.Database.Driver)
	}

	db, err := sql.Open("mysql", bc.Data.Database.Source)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if err := goose.SetDialect("mysql"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}

	if logger != nil {
		logger.Info("run database migration", "command", command, "dir", migrationsDir)
	}
	if err := goose.Run(command, db, migrationsDir, args...); err != nil {
		return fmt.Errorf("run migration %s: %w", command, err)
	}
	return nil
}

func validateDatabaseConfig(bc *conf.Bootstrap) error {
	if bc == nil || bc.Data == nil || bc.Data.Database == nil {
		return fmt.Errorf("data.database: missing")
	}
	if strings.TrimSpace(bc.Data.Database.Source) == "" {
		return fmt.Errorf("data.database.source: required")
	}
	return nil
}
