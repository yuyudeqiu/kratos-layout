package data

import (
	"time"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewTaskRepo)

// Data .
type Data struct {
	db *gorm.DB
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
	helper.Info("connected to PostgreSQL")

	cleanup := func() {
		helper.Info("closing the data resources")
		sqlDB, err := db.DB()
		if err != nil {
			helper.Errorf("failed to get sql.DB: %v", err)
			return
		}
		if err := sqlDB.Close(); err != nil {
			helper.Errorf("failed to close db: %v", err)
		}
	}
	return &Data{db: db}, cleanup, nil
}
