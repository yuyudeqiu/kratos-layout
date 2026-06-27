package data

import (
	"context"
	"strings"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"

	"gorm.io/gorm"
)

// TodoModel is the GORM model for the todos table.
type TodoModel struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	Title      string    `gorm:"type:varchar(255);not null"`
	Content    string    `gorm:"type:text"`
	Completed  bool      `gorm:"default:false"`
	CreateTime time.Time `gorm:"autoCreateTime"`
	UpdateTime time.Time `gorm:"autoUpdateTime"`
}

// TableName overrides the default table name.
func (TodoModel) TableName() string {
	return "todos"
}

type todoRepo struct {
	db *gorm.DB
}

// NewTodoRepo creates a new TodoRepo instance.
func NewTodoRepo(data *Data) biz.TodoRepo {
	return &todoRepo{db: data.DB}
}

func (r *todoRepo) FindByID(ctx context.Context, id int64) (*biz.Todo, error) {
	var model TodoModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, biz.ErrTodoNotFound
		}
		return nil, err
	}
	return model.toBiz(), nil
}

func (r *todoRepo) ListTodos(ctx context.Context, opts ...biz.ListOption) ([]*biz.Todo, error) {
	options := biz.ListOptions{Limit: 20}
	for _, opt := range opts {
		opt(&options)
	}
	if options.Offset < 0 || options.Limit <= 0 {
		return nil, biz.ErrTodoInvalidArgument
	}

	db := r.db.WithContext(ctx)
	if options.Completed != nil {
		db = db.Where("completed = ?", *options.Completed)
	}
	if options.Search != "" {
		db = db.Where("title LIKE ? OR content LIKE ?", "%"+options.Search+"%", "%"+options.Search+"%")
	}
	if options.OrderBy != "" {
		switch strings.ToLower(options.OrderBy) {
		case "id asc", "id desc", "title asc", "title desc", "create_time asc", "create_time desc", "update_time asc", "update_time desc":
			db = db.Order(options.OrderBy)
		default:
			db = db.Order("id ASC")
		}
	} else {
		db = db.Order("id ASC")
	}

	var models []TodoModel
	if err := db.
		Offset(options.Offset).
		Limit(options.Limit).
		Find(&models).Error; err != nil {
		return nil, err
	}

	todos := make([]*biz.Todo, 0, len(models))
	for _, m := range models {
		todos = append(todos, m.toBiz())
	}
	return todos, nil
}

func (r *todoRepo) CreateTodo(ctx context.Context, todo *biz.Todo) (*biz.Todo, error) {
	model := fromBizTodo(todo)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return nil, err
	}
	return model.toBiz(), nil
}

func (r *todoRepo) UpdateTodo(ctx context.Context, todo *biz.Todo) (*biz.Todo, error) {
	var model TodoModel
	if err := r.db.WithContext(ctx).First(&model, todo.ID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, biz.ErrTodoNotFound
		}
		return nil, err
	}
	// Only update mutable fields, preserve CreateTime.
	updates := map[string]any{
		"title":     todo.Title,
		"content":   todo.Content,
		"completed": todo.Completed,
	}
	if err := r.db.WithContext(ctx).Model(&model).Updates(updates).Error; err != nil {
		return nil, err
	}
	// Re-fetch to get the updated record with UpdateTime.
	if err := r.db.WithContext(ctx).First(&model, todo.ID).Error; err != nil {
		return nil, err
	}
	return model.toBiz(), nil
}

func (r *todoRepo) DeleteTodo(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&TodoModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return biz.ErrTodoNotFound
	}
	return nil
}

// toBiz converts a TodoModel to a biz.Todo.
func (m TodoModel) toBiz() *biz.Todo {
	return &biz.Todo{
		ID:         m.ID,
		Title:      m.Title,
		Content:    m.Content,
		Completed:  m.Completed,
		CreateTime: m.CreateTime,
		UpdateTime: m.UpdateTime,
	}
}

// fromBizTodo converts a biz.Todo to a TodoModel.
func fromBizTodo(todo *biz.Todo) TodoModel {
	if todo == nil {
		return TodoModel{}
	}
	return TodoModel{
		ID:        todo.ID,
		Title:     todo.Title,
		Content:   todo.Content,
		Completed: todo.Completed,
	}
}
