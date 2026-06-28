package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	v1 "github.com/go-kratos/kratos-layout/api/todo/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"

	"github.com/go-kratos/kratos/v3/log"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
	"go.einride.tech/aip/pagination"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// TodoService is a todo service.
type TodoService struct {
	v1.UnimplementedTodoServiceServer

	uc *biz.TodoUsecase
}

// NewTodoService new a todo service.
func NewTodoService(uc *biz.TodoUsecase) *TodoService {
	return &TodoService{uc: uc}
}

// CreateTodo creates a todo item.
func (s *TodoService) CreateTodo(ctx context.Context, req *v1.CreateTodoRequest) (*v1.Todo, error) {
	log.InfoContext(ctx, "CreateTodo called", "title", req.GetTodo().GetTitle())
	todo, err := s.uc.CreateTodo(ctx, convertTodo(req.GetTodo()))
	if err != nil {
		return nil, err
	}
	return convertTodoReply(todo), nil
}

// GetTodo returns a todo item by ID.
func (s *TodoService) GetTodo(ctx context.Context, req *v1.GetTodoRequest) (*v1.Todo, error) {
	log.InfoContext(ctx, "GetTodo called", "id", req.GetId())
	todo, err := s.uc.GetTodo(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return convertTodoReply(todo), nil
}

// ListTodos lists todo items.
func (s *TodoService) ListTodos(ctx context.Context, req *v1.ListTodosRequest) (*v1.TodoSet, error) {
	log.InfoContext(ctx, "ListTodos called", "page_size", req.PageSize, "page_token", req.GetPageToken())
	pageToken, err := normalizeListTodosRequest(req)
	if err != nil {
		return nil, err
	}

	todos, err := s.uc.ListTodos(ctx,
		biz.ListFilter(req.GetFilter()),
		biz.ListOrderBy(req.GetOrderBy()),
		biz.ListLimit(int(req.PageSize)+1),
		biz.ListOffset(int(pageToken.Offset)),
	)
	if err != nil {
		return nil, err
	}

	hasNextPage := len(todos) > int(req.GetPageSize())
	if hasNextPage {
		todos = todos[:req.GetPageSize()]
	}
	set := &v1.TodoSet{
		Todos: make([]*v1.Todo, 0, len(todos)),
	}
	for _, todo := range todos {
		set.Todos = append(set.Todos, convertTodoReply(todo))
	}
	if hasNextPage {
		set.NextPageToken = pageToken.Next(req).String()
	}
	return set, nil
}

// UpdateTodo updates a todo item.
func (s *TodoService) UpdateTodo(ctx context.Context, req *v1.UpdateTodoRequest) (*v1.Todo, error) {
	log.InfoContext(ctx, "UpdateTodo called", "id", req.GetTodo().GetId())
	if req.GetTodo().GetId() <= 0 || req.GetUpdateMask() == nil || len(req.GetUpdateMask().GetPaths()) == 0 {
		return nil, biz.ErrTodoInvalidArgument
	}
	current, err := s.GetTodo(ctx, &v1.GetTodoRequest{Id: req.GetTodo().GetId()})
	if err != nil {
		return nil, err
	}
	fieldmask.Update(req.GetUpdateMask(), current, req.GetTodo())
	todo, err := s.uc.UpdateTodo(ctx, convertTodo(current))
	if err != nil {
		return nil, err
	}
	return convertTodoReply(todo), nil
}

// DeleteTodo deletes a todo item.
func (s *TodoService) DeleteTodo(ctx context.Context, req *v1.DeleteTodoRequest) (*emptypb.Empty, error) {
	log.InfoContext(ctx, "DeleteTodo called", "id", req.GetId())
	if err := s.uc.DeleteTodo(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// WatchTodos streams todo snapshots from the server to the client.
func (s *TodoService) WatchTodos(req *v1.WatchTodosRequest, stream v1.TodoService_WatchTodosServer) error {
	log.InfoContext(stream.Context(), "WatchTodos called", "page_size", req.PageSize)
	if req.PageSize <= 0 {
		req.PageSize = defaultPageSize
	} else if req.PageSize > maxPageSize {
		req.PageSize = maxPageSize
	}

	var completed *bool
	if req.Completed != nil {
		c := req.GetCompleted()
		completed = &c
	}

	todos, err := s.uc.ListTodos(stream.Context(),
		biz.ListCompleted(completed),
		biz.ListSearch(req.GetSearch()),
		biz.ListOrderBy(req.GetOrderBy()),
		biz.ListLimit(int(req.PageSize)),
		biz.ListOffset(int(req.GetOffset())),
	)
	if err != nil {
		return err
	}
	for _, todo := range todos {
		if err := stream.Send(newTodoEvent("snapshot", todo)); err != nil {
			return err
		}
	}
	return nil
}

// SyncTodos exchanges todo changes in both directions.
func (s *TodoService) SyncTodos(stream v1.TodoService_SyncTodosServer) error {
	log.InfoContext(stream.Context(), "SyncTodos started")
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var event *v1.TodoEvent
		switch strings.ToLower(req.GetAction()) {
		case "create":
			todo, err := s.CreateTodo(stream.Context(), &v1.CreateTodoRequest{Todo: req.GetTodo()})
			if err != nil {
				return err
			}
			event = newTodoEvent("created", convertTodo(todo))
		case "update":
			todo, err := s.UpdateTodo(stream.Context(), &v1.UpdateTodoRequest{
				Todo:       req.GetTodo(),
				UpdateMask: req.GetUpdateMask(),
			})
			if err != nil {
				return err
			}
			event = newTodoEvent("updated", convertTodo(todo))
		case "delete":
			id := req.GetId()
			if id == 0 {
				id = req.GetTodo().GetId()
			}
			if _, err := s.DeleteTodo(stream.Context(), &v1.DeleteTodoRequest{Id: id}); err != nil {
				return err
			}
			event = &v1.TodoEvent{
				Action:    "deleted",
				Todo:      &v1.Todo{Id: id},
				EventTime: timestamppb.Now(),
			}
		default:
			return biz.ErrTodoInvalidArgument
		}
		if err := stream.Send(event); err != nil {
			return err
		}
	}
}

func normalizeListTodosRequest(req *v1.ListTodosRequest) (pagination.PageToken, error) {
	switch {
	case req.GetPageSize() < 0:
		return pagination.PageToken{}, biz.ErrTodoInvalidArgument
	case req.GetPageSize() == 0:
		req.PageSize = defaultPageSize
	case req.GetPageSize() > maxPageSize:
		req.PageSize = maxPageSize
	}

	if _, err := parseTodoOrderBy(req); err != nil {
		return pagination.PageToken{}, biz.ErrTodoInvalidArgument
	}
	if _, err := parseTodoFilter(req); err != nil {
		return pagination.PageToken{}, biz.ErrTodoInvalidArgument
	}
	pageToken, err := pagination.ParsePageToken(req)
	if err != nil {
		return pagination.PageToken{}, biz.ErrTodoInvalidArgument
	}
	return pageToken, nil
}

func parseTodoOrderBy(req *v1.ListTodosRequest) (ordering.OrderBy, error) {
	orderBy, err := ordering.ParseOrderBy(req)
	if err != nil {
		return ordering.OrderBy{}, err
	}
	if err := orderBy.ValidateForPaths("id", "title", "completed", "create_time", "update_time"); err != nil {
		return ordering.OrderBy{}, err
	}
	return orderBy, nil
}

func parseTodoFilter(req *v1.ListTodosRequest) (filtering.Filter, error) {
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
		return filtering.Filter{}, err
	}
	return filtering.ParseFilter(req, declarations)
}

func convertTodo(in *v1.Todo) *biz.Todo {
	if in == nil {
		return nil
	}
	return &biz.Todo{
		ID:        in.GetId(),
		Title:     in.GetTitle(),
		Content:   in.GetContent(),
		Completed: in.GetCompleted(),
	}
}

func newTodoEvent(action string, todo *biz.Todo) *v1.TodoEvent {
	return &v1.TodoEvent{
		Action:    action,
		Todo:      convertTodoReply(todo),
		EventTime: timestamppb.New(time.Now()),
	}
}

func convertTodoReply(in *biz.Todo) *v1.Todo {
	if in == nil {
		return nil
	}
	return &v1.Todo{
		Id:         in.ID,
		Title:      in.Title,
		Content:    in.Content,
		Completed:  in.Completed,
		CreateTime: timestamppb.New(in.CreateTime),
		UpdateTime: timestamppb.New(in.UpdateTime),
	}
}
