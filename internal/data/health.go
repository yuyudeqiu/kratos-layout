package data

import (
	"context"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos/v2/log"
)

type healthRepo struct {
	data *Data
	log  *log.Helper
}

// NewHealthRepo new a health repo.
func NewHealthRepo(data *Data, logger log.Logger) biz.HealthRepo {
	return &healthRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

// Ping checks if SQL database and Redis are reachable.
func (r *healthRepo) Ping(ctx context.Context) error {
	// Ping database
	sqlDB, err := r.data.db.DB()
	if err != nil {
		r.log.WithContext(ctx).Errorf("failed to get sql.DB for healthcheck: %v", err)
		return err
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		r.log.WithContext(ctx).Errorf("database ping failed: %v", err)
		return err
	}

	// Ping Redis (if initialized)
	if r.data.rdb != nil {
		if err := r.data.rdb.Ping(ctx).Err(); err != nil {
			r.log.WithContext(ctx).Errorf("redis ping failed: %v", err)
			return err
		}
	}

	return nil
}
