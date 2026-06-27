package data

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewTodoRepo, ProvideSQLDB)

// ProvideSQLDB extracts the underlying *sql.DB from GORM for health checks etc.
func ProvideSQLDB(d *Data) *sql.DB {
	sqlDB, err := d.DB.DB()
	if err != nil {
		panic(err) // never happens after successful NewData
	}
	return sqlDB
}

// Data holds shared data resources (e.g. DB, Redis, MQ clients).
type Data struct {
	DB *gorm.DB
}

// NewData creates a Data instance and connects to the database.
func NewData(c *conf.Data, logger *slog.Logger) (*Data, func(), error) {
	gormLogger := gormlog.NewSlogLogger(
		logger,
		gormlog.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormLogLevel(logger),
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		},
	)
	db, err := gorm.Open(mysql.Open(c.Database.Source), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, nil, err
	}
	// Register OpenTelemetry tracing plugin so each DB operation creates a
	// child span (e.g. "gorm.query", "gorm.create") under the request span.
	if err := db.Use(tracing.NewPlugin()); err != nil {
		return nil, nil, err
	}
	if err := db.AutoMigrate(&TodoModel{}); err != nil {
		return nil, nil, err
	}
	d := &Data{DB: db}
	cleanup := func() {
		log.Info("closing the data resources")
		sqlDB, err := db.DB()
		if err != nil {
			log.Error("get sql.DB failed: ", err)
			return
		}
		if err := sqlDB.Close(); err != nil {
			log.Error("close database failed: ", err)
		}
	}
	return d, cleanup, nil
}

// gormLogLevel maps the slog logger's level to GORM's log level.
// When the app runs at debug, GORM emits Info (SQL queries).
// When the app runs at info/warn/error, GORM only emits Warn (slow queries + errors).
func gormLogLevel(logger *slog.Logger) gormlog.LogLevel {
	if logger.Handler().Enabled(context.Background(), slog.LevelDebug) {
		return gormlog.Info
	}
	return gormlog.Warn
}
