package data

import (
	"context"

	"github.com/go-kratos/kratos-layout/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
)

// Greeter is the GORM model (PO - Persistence Object) for the greeters table.
type Greeter struct {
	gorm.Model
	Hello string `gorm:"column:hello;type:varchar(255);not null"`
}

type greeterRepo struct {
	data *Data
	log  *log.Helper
}

// NewGreeterRepo .
func NewGreeterRepo(data *Data, logger log.Logger) biz.GreeterRepo {
	// Auto-migrate table schema in dev.
	if err := data.db.AutoMigrate(&Greeter{}); err != nil {
		log.NewHelper(logger).Fatalf("failed to auto migrate greeter: %v", err)
	}
	return &greeterRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (r *greeterRepo) Save(ctx context.Context, g *biz.Greeter) (*biz.Greeter, error) {
	po := &Greeter{Hello: g.Hello}
	if err := r.data.db.WithContext(ctx).Create(po).Error; err != nil {
		return nil, err
	}
	return &biz.Greeter{Hello: po.Hello}, nil
}

func (r *greeterRepo) Update(ctx context.Context, g *biz.Greeter) (*biz.Greeter, error) {
	if err := r.data.db.WithContext(ctx).Model(&Greeter{}).
		Where("hello = ?", g.Hello).
		Updates(map[string]interface{}{"hello": g.Hello}).Error; err != nil {
		return nil, err
	}
	return g, nil
}

func (r *greeterRepo) FindByID(ctx context.Context, id int64) (*biz.Greeter, error) {
	var po Greeter
	if err := r.data.db.WithContext(ctx).First(&po, id).Error; err != nil {
		return nil, err
	}
	return &biz.Greeter{Hello: po.Hello}, nil
}

func (r *greeterRepo) ListByHello(ctx context.Context, hello string) ([]*biz.Greeter, error) {
	var pos []Greeter
	if err := r.data.db.WithContext(ctx).Where("hello = ?", hello).Find(&pos).Error; err != nil {
		return nil, err
	}
	result := make([]*biz.Greeter, len(pos))
	for i, po := range pos {
		result[i] = &biz.Greeter{Hello: po.Hello}
	}
	return result, nil
}

func (r *greeterRepo) ListAll(ctx context.Context) ([]*biz.Greeter, error) {
	var pos []Greeter
	if err := r.data.db.WithContext(ctx).Find(&pos).Error; err != nil {
		return nil, err
	}
	result := make([]*biz.Greeter, len(pos))
	for i, po := range pos {
		result[i] = &biz.Greeter{Hello: po.Hello}
	}
	return result, nil
}
