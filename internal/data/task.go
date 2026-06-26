package data

import (
	"context"
	"errors"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
)

// Task is the GORM model (PO) for the tasks table.
type Task struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	Title     string `gorm:"type:varchar(255);not null"`
	Content   string `gorm:"type:text"`
	Status    string `gorm:"type:varchar(50);default:'pending'"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// ToDomain maps persistence object to domain model.
func (t *Task) ToDomain() *biz.Task {
	return &biz.Task{
		ID:        t.ID,
		Title:     t.Title,
		Content:   t.Content,
		Status:    t.Status,
		CreatedAt: t.CreatedAt.Unix(),
		UpdatedAt: t.UpdatedAt.Unix(),
	}
}

type taskRepo struct {
	data *Data
	log  *log.Helper
}

// NewTaskRepo .
func NewTaskRepo(data *Data, logger log.Logger) biz.TaskRepo {
	// Auto-migrate table schema in dev.
	if err := data.db.AutoMigrate(&Task{}); err != nil {
		log.NewHelper(logger).Fatalf("failed to auto migrate task: %v", err)
	}
	return &taskRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (r *taskRepo) Create(ctx context.Context, t *biz.Task) (*biz.Task, error) {
	po := &Task{
		Title:   t.Title,
		Content: t.Content,
		Status:  t.Status,
	}
	if po.Status == "" {
		po.Status = "pending"
	}
	if err := r.data.db.WithContext(ctx).Create(po).Error; err != nil {
		return nil, err
	}
	return po.ToDomain(), nil
}

func (r *taskRepo) Update(ctx context.Context, t *biz.Task) (*biz.Task, error) {
	po := &Task{
		ID:      t.ID,
		Title:   t.Title,
		Content: t.Content,
		Status:  t.Status,
	}
	if err := r.data.db.WithContext(ctx).Model(po).Updates(map[string]interface{}{
		"title":   po.Title,
		"content": po.Content,
		"status":  po.Status,
	}).Error; err != nil {
		return nil, err
	}
	return r.Get(ctx, t.ID)
}

func (r *taskRepo) Get(ctx context.Context, id int64) (*biz.Task, error) {
	var po Task
	if err := r.data.db.WithContext(ctx).First(&po, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, biz.ErrTaskNotFound
		}
		return nil, err
	}
	return po.ToDomain(), nil
}

func (r *taskRepo) List(ctx context.Context) ([]*biz.Task, error) {
	var pos []Task
	if err := r.data.db.WithContext(ctx).Order("id desc").Find(&pos).Error; err != nil {
		return nil, err
	}
	result := make([]*biz.Task, len(pos))
	for i, po := range pos {
		result[i] = po.ToDomain()
	}
	return result, nil
}

func (r *taskRepo) Delete(ctx context.Context, id int64) error {
	if err := r.data.db.WithContext(ctx).Delete(&Task{}, id).Error; err != nil {
		return err
	}
	return nil
}
