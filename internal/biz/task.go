package biz

import (
	"context"

	v1 "github.com/go-kratos/kratos-layout/api/task/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

var (
	// ErrTaskNotFound is task not found.
	ErrTaskNotFound = errors.NotFound(v1.ErrorReason_TASK_NOT_FOUND.String(), "task not found")
)

// Task is a Task domain model.
type Task struct {
	ID        int64
	Title     string
	Content   string
	Status    string
	CreatedAt int64
	UpdatedAt int64
}

// TaskRepo is a Task repo interface.
type TaskRepo interface {
	Create(context.Context, *Task) (*Task, error)
	Update(context.Context, *Task) (*Task, error)
	Get(context.Context, int64) (*Task, error)
	List(context.Context) ([]*Task, error)
	Delete(context.Context, int64) error
}

// TaskUsecase is a Task usecase.
type TaskUsecase struct {
	repo TaskRepo
	log  *log.Helper
}

// NewTaskUsecase new a Task usecase.
func NewTaskUsecase(repo TaskRepo, logger log.Logger) *TaskUsecase {
	return &TaskUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// CreateTask creates a task.
func (uc *TaskUsecase) CreateTask(ctx context.Context, t *Task) (*Task, error) {
	uc.log.WithContext(ctx).Infof("CreateTask: %v", t.Title)
	return uc.repo.Create(ctx, t)
}

// UpdateTask updates a task.
func (uc *TaskUsecase) UpdateTask(ctx context.Context, t *Task) (*Task, error) {
	uc.log.WithContext(ctx).Infof("UpdateTask: %d", t.ID)
	return uc.repo.Update(ctx, t)
}

// GetTask gets a task by ID.
func (uc *TaskUsecase) GetTask(ctx context.Context, id int64) (*Task, error) {
	uc.log.WithContext(ctx).Infof("GetTask: %d", id)
	return uc.repo.Get(ctx, id)
}

// ListTasks lists all tasks.
func (uc *TaskUsecase) ListTasks(ctx context.Context) ([]*Task, error) {
	uc.log.WithContext(ctx).Info("ListTasks")
	return uc.repo.List(ctx)
}

// DeleteTask deletes a task by ID.
func (uc *TaskUsecase) DeleteTask(ctx context.Context, id int64) error {
	uc.log.WithContext(ctx).Infof("DeleteTask: %d", id)
	return uc.repo.Delete(ctx, id)
}
