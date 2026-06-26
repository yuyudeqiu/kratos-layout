package data

import (
	"time"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewTaskRepo, NewHealthRepo)

// Data .
type Data struct {
	db  *gorm.DB
	rdb *redis.Client
}

// NewData .
func NewData(c *conf.Data, logger log.Logger) (*Data, func(), error) {
	helper := log.NewHelper(logger)

	db, err := gorm.Open(postgres.Open(c.Database.Source), &gorm.Config{
		Logger: NewGormLogger(logger, 200*time.Millisecond),
	})
	if err != nil {
		return nil, nil, err
	}

	if err := db.Use(tracing.NewPlugin()); err != nil {
		return nil, nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	if c.Database.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(int(c.Database.MaxIdleConns))
	}
	if c.Database.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(int(c.Database.MaxOpenConns))
	}
	if c.Database.ConnMaxLifetime != nil {
		sqlDB.SetConnMaxLifetime(c.Database.ConnMaxLifetime.AsDuration())
	}

	var rdb *redis.Client
	if c.Redis != nil {
		opts := &redis.Options{
			Addr: c.Redis.Addr,
		}
		if c.Redis.Network != "" {
			opts.Network = c.Redis.Network
		}
		if c.Redis.ReadTimeout != nil {
			opts.ReadTimeout = c.Redis.ReadTimeout.AsDuration()
		}
		if c.Redis.WriteTimeout != nil {
			opts.WriteTimeout = c.Redis.WriteTimeout.AsDuration()
		}
		rdb = redis.NewClient(opts)
	}

	helper.Info("connected to PostgreSQL and initialized Redis")

	cleanup := func() {
		helper.Info("closing the data resources")
		sqlDB, err := db.DB()
		if err != nil {
			helper.Errorf("failed to get sql.DB: %v", err)
		} else {
			if err := sqlDB.Close(); err != nil {
				helper.Errorf("failed to close db: %v", err)
			}
		}
		if rdb != nil {
			if err := rdb.Close(); err != nil {
				helper.Errorf("failed to close redis: %v", err)
			}
		}
	}
	return &Data{db: db, rdb: rdb}, cleanup, nil
}
