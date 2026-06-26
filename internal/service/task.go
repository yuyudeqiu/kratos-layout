package service

import (
	"context"

	pb "github.com/go-kratos/kratos-layout/api/task/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
)

type TaskService struct {
	pb.UnimplementedTaskServer
	uc *biz.TaskUsecase
}

func NewTaskService(uc *biz.TaskUsecase) *TaskService {
	return &TaskService{uc: uc}
}

func mapToProto(t *biz.Task) *pb.TaskInfo {
	if t == nil {
		return nil
	}
	return &pb.TaskInfo{
		Id:        t.ID,
		Title:     t.Title,
		Content:   t.Content,
		Status:    t.Status,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

func (s *TaskService) CreateTask(ctx context.Context, req *pb.CreateTaskRequest) (*pb.CreateTaskReply, error) {
	t, err := s.uc.CreateTask(ctx, &biz.Task{
		Title:   req.Title,
		Content: req.Content,
	})
	if err != nil {
		return nil, err
	}
	return &pb.CreateTaskReply{
		Task: mapToProto(t),
	}, nil
}

func (s *TaskService) UpdateTask(ctx context.Context, req *pb.UpdateTaskRequest) (*pb.UpdateTaskReply, error) {
	t, err := s.uc.UpdateTask(ctx, &biz.Task{
		ID:      req.Id,
		Title:   req.Title,
		Content: req.Content,
		Status:  req.Status,
	})
	if err != nil {
		return nil, err
	}
	return &pb.UpdateTaskReply{
		Task: mapToProto(t),
	}, nil
}

func (s *TaskService) DeleteTask(ctx context.Context, req *pb.DeleteTaskRequest) (*pb.DeleteTaskReply, error) {
	err := s.uc.DeleteTask(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &pb.DeleteTaskReply{
		Success: true,
	}, nil
}

func (s *TaskService) GetTask(ctx context.Context, req *pb.GetTaskRequest) (*pb.GetTaskReply, error) {
	t, err := s.uc.GetTask(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &pb.GetTaskReply{
		Task: mapToProto(t),
	}, nil
}

func (s *TaskService) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksReply, error) {
	tasks, err := s.uc.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	res := make([]*pb.TaskInfo, len(tasks))
	for i, t := range tasks {
		res[i] = mapToProto(t)
	}
	return &pb.ListTasksReply{
		Tasks: res,
	}, nil
}
