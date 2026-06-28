package data

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/go-kratos/kratos-layout/api/todo/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"

	"go.einride.tech/aip/filtering"
	aipordering "go.einride.tech/aip/ordering"
	googleexpr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
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

// NewTodoRepo creates a new TodoRepo instance (uncached, for tests).
func NewTodoRepo(data *Data) biz.TodoRepo {
	return &todoRepo{db: data.DB}
}

// NewCachedTodoRepo creates a new TodoRepo instance with Redis cache-aside.
func NewCachedTodoRepo(data *Data) biz.TodoRepo {
	return &cachedTodoRepo{
		TodoRepo: &todoRepo{db: data.DB},
		redis:    data.Redis,
	}
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
	var err error
	if options.Filter != "" {
		db, err = applyTodoFilter(db, options.Filter)
		if err != nil {
			return nil, err
		}
	}
	if options.OrderBy != "" {
		db, err = applyTodoOrderBy(db, options.OrderBy)
		if err != nil {
			return nil, err
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

func applyTodoOrderBy(db *gorm.DB, orderBy string) (*gorm.DB, error) {
	parsed, err := aipordering.ParseOrderBy(todoListRequest{orderBy: orderBy})
	if err != nil {
		return nil, biz.ErrTodoInvalidArgument
	}
	if err := parsed.ValidateForPaths("id", "title", "completed", "create_time", "update_time"); err != nil {
		return nil, biz.ErrTodoInvalidArgument
	}
	for _, field := range parsed.Fields {
		column, ok := todoOrderColumns[field.Path]
		if !ok {
			return nil, biz.ErrTodoInvalidArgument
		}
		direction := "ASC"
		if field.Desc {
			direction = "DESC"
		}
		db = db.Order(fmt.Sprintf("%s %s", column, direction))
	}
	return db, nil
}

func applyTodoFilter(db *gorm.DB, filter string) (*gorm.DB, error) {
	declarationOptions := []filtering.DeclarationOption{filtering.DeclareStandardFunctions()}
	declarationOptions = append(
		declarationOptions,
		filtering.DeclareProtoMessageIdents(&v1.Todo{}, filtering.WithFilterableFields(
			"id",
			"title",
			"content",
			"completed",
			"create_time",
			"update_time",
		))...,
	)
	declarations, err := filtering.NewDeclarations(declarationOptions...)
	if err != nil {
		return nil, err
	}
	parsed, err := filtering.ParseFilter(todoListRequest{filter: filter}, declarations)
	if err != nil {
		return nil, biz.ErrTodoInvalidArgument
	}
	return applyTodoFilterExpr(db, parsed.CheckedExpr.GetExpr())
}

func applyTodoFilterExpr(db *gorm.DB, e *googleexpr.Expr) (*gorm.DB, error) {
	if field, ok := todoFilterBoolField(e); ok {
		return db.Where(todoFilterColumns[field]+" = ?", true), nil
	}
	call := e.GetCallExpr()
	if call == nil {
		return nil, biz.ErrTodoInvalidArgument
	}
	args := call.GetArgs()
	switch call.GetFunction() {
	case filtering.FunctionAnd:
		if len(args) != 2 {
			return nil, biz.ErrTodoInvalidArgument
		}
		var err error
		db, err = applyTodoFilterExpr(db, args[0])
		if err != nil {
			return nil, err
		}
		return applyTodoFilterExpr(db, args[1])
	case filtering.FunctionOr:
		if len(args) != 2 {
			return nil, biz.ErrTodoInvalidArgument
		}
		left, err := buildTodoPredicate(args[0])
		if err != nil {
			return nil, err
		}
		right, err := buildTodoPredicate(args[1])
		if err != nil {
			return nil, err
		}
		return db.Where(db.Where(left.query, left.args...).Or(right.query, right.args...)), nil
	case filtering.FunctionNot:
		if len(args) != 1 {
			return nil, biz.ErrTodoInvalidArgument
		}
		field, ok := todoFilterBoolField(args[0])
		if !ok {
			return nil, biz.ErrTodoInvalidArgument
		}
		return db.Where(todoFilterColumns[field]+" = ?", false), nil
	default:
		predicate, err := buildTodoPredicate(e)
		if err != nil {
			return nil, err
		}
		return db.Where(predicate.query, predicate.args...), nil
	}
}

type todoPredicate struct {
	query string
	args  []any
}

func buildTodoPredicate(e *googleexpr.Expr) (todoPredicate, error) {
	call := e.GetCallExpr()
	if call == nil || len(call.GetArgs()) != 2 {
		return todoPredicate{}, biz.ErrTodoInvalidArgument
	}

	field, ok := todoFilterField(call.GetArgs()[0])
	if !ok {
		return todoPredicate{}, biz.ErrTodoInvalidArgument
	}
	column, ok := todoFilterColumns[field]
	if !ok {
		return todoPredicate{}, biz.ErrTodoInvalidArgument
	}
	value, err := todoFilterValue(field, call.GetArgs()[1])
	if err != nil {
		return todoPredicate{}, err
	}

	switch call.GetFunction() {
	case filtering.FunctionEquals:
		return todoPredicate{query: column + " = ?", args: []any{value}}, nil
	case filtering.FunctionNotEquals:
		return todoPredicate{query: column + " <> ?", args: []any{value}}, nil
	case filtering.FunctionLessThan:
		return todoPredicate{query: column + " < ?", args: []any{value}}, nil
	case filtering.FunctionLessEquals:
		return todoPredicate{query: column + " <= ?", args: []any{value}}, nil
	case filtering.FunctionGreaterThan:
		return todoPredicate{query: column + " > ?", args: []any{value}}, nil
	case filtering.FunctionGreaterEquals:
		return todoPredicate{query: column + " >= ?", args: []any{value}}, nil
	case filtering.FunctionHas:
		if field != "title" && field != "content" {
			return todoPredicate{}, biz.ErrTodoInvalidArgument
		}
		s, ok := value.(string)
		if !ok {
			return todoPredicate{}, biz.ErrTodoInvalidArgument
		}
		return todoPredicate{query: column + " LIKE ?", args: []any{"%" + s + "%"}}, nil
	default:
		return todoPredicate{}, biz.ErrTodoInvalidArgument
	}
}

func todoFilterField(e *googleexpr.Expr) (string, bool) {
	ident := e.GetIdentExpr()
	if ident == nil {
		return "", false
	}
	return ident.GetName(), true
}

func todoFilterBoolField(e *googleexpr.Expr) (string, bool) {
	field, ok := todoFilterField(e)
	return field, ok && field == "completed"
}

func todoFilterValue(field string, e *googleexpr.Expr) (any, error) {
	c := e.GetConstExpr()
	if c == nil {
		return nil, biz.ErrTodoInvalidArgument
	}
	switch field {
	case "id":
		return c.GetInt64Value(), nil
	case "title", "content":
		return c.GetStringValue(), nil
	case "completed":
		return c.GetBoolValue(), nil
	case "create_time", "update_time":
		t, err := time.Parse(time.RFC3339, c.GetStringValue())
		if err != nil {
			return nil, biz.ErrTodoInvalidArgument
		}
		return t, nil
	default:
		return nil, biz.ErrTodoInvalidArgument
	}
}

type todoListRequest struct {
	filter  string
	orderBy string
}

func (r todoListRequest) GetFilter() string {
	return r.filter
}

func (r todoListRequest) GetOrderBy() string {
	return r.orderBy
}

var todoFilterColumns = map[string]string{
	"id":          "id",
	"title":       "title",
	"content":     "content",
	"completed":   "completed",
	"create_time": "create_time",
	"update_time": "update_time",
}

var todoOrderColumns = map[string]string{
	"id":          "id",
	"title":       "title",
	"completed":   "completed",
	"create_time": "create_time",
	"update_time": "update_time",
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
