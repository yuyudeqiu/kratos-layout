package data

import (
	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewTodoRepo)

// Data holds shared data resources (e.g. DB, Redis, MQ clients).
type Data struct {
	DB *gorm.DB
}

// NewData creates a Data instance and connects to the database.
func NewData(c *conf.Data) (*Data, func(), error) {
	db, err := gorm.Open(mysql.Open(c.Database.Source), &gorm.Config{})
	if err != nil {
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
