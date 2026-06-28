package data

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewCachedTodoRepo, ProvideSQLDB, ProvideRedisClient)

// ProvideSQLDB extracts the underlying *sql.DB from GORM for health checks etc.
func ProvideSQLDB(d *Data) *sql.DB {
	sqlDB, err := d.DB.DB()
	if err != nil {
		panic(err) // never happens after successful NewData
	}
	return sqlDB
}

// ProvideRedisClient extracts the Redis client for health checks etc.
func ProvideRedisClient(d *Data) redis.UniversalClient {
	return d.Redis
}

// Data holds shared data resources (e.g. DB, Redis, MQ clients).
type Data struct {
	DB    *gorm.DB
	Redis redis.UniversalClient
}

// NewData creates a Data instance and connects to the database and Redis.
func NewData(c *conf.Data, logger *slog.Logger) (*Data, func(), error) {
	// --- Database ---
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

	// Configure connection pool. Unset config values use code-level defaults.
	{
		sqlDB, err := db.DB()
		if err != nil {
			return nil, nil, err
		}
		maxOpen := 25
		maxIdle := 10
		connMaxLifetime := 5 * time.Minute
		connMaxIdleTime := 1 * time.Minute
		if v := int(c.Database.MaxOpenConns); v > 0 {
			maxOpen = v
		}
		if v := int(c.Database.MaxIdleConns); v > 0 {
			maxIdle = v
		}
		if d := c.Database.ConnMaxLifetime; d != nil {
			connMaxLifetime = d.AsDuration()
		}
		if d := c.Database.ConnMaxIdleTime; d != nil {
			connMaxIdleTime = d.AsDuration()
		}
		sqlDB.SetMaxOpenConns(maxOpen)
		sqlDB.SetMaxIdleConns(maxIdle)
		sqlDB.SetConnMaxLifetime(connMaxLifetime)
		sqlDB.SetConnMaxIdleTime(connMaxIdleTime)
	}

	if err := db.AutoMigrate(&TodoModel{}); err != nil {
		return nil, nil, err
	}

	// --- Redis ---
	rdb := redis.NewClient(&redis.Options{
		Network:      c.Redis.Network,
		Addr:         c.Redis.Addr,
		Password:     c.Redis.Password,
		DB:           int(c.Redis.Db),
		DialTimeout:  c.Redis.DialTimeout.AsDuration(),
		ReadTimeout:  c.Redis.ReadTimeout.AsDuration(),
		WriteTimeout: c.Redis.WriteTimeout.AsDuration(),
		PoolSize:     int(c.Redis.PoolSize),
	})

	// Enable OpenTelemetry tracing for Redis commands.
	// Each Redis operation creates a child span (e.g. "SET", "GET") under the
	// request span, with attributes like db.system=redis, db.statement, etc.
	if err := redisotel.InstrumentTracing(rdb); err != nil {
		return nil, nil, err
	}

	d := &Data{DB: db, Redis: rdb}

	cleanup := func() {
		log.Info("closing the data resources")
		if err := rdb.Close(); err != nil {
			log.Error("close redis failed: ", err)
		}
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
