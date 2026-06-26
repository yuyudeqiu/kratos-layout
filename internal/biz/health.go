package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
)

// HealthRepo is Health repo interface.
type HealthRepo interface {
	Ping(ctx context.Context) error
}

// HealthUsecase is Health usecase.
type HealthUsecase struct {
	repo HealthRepo
	log  *log.Helper
}

// NewHealthUsecase new a Health usecase.
func NewHealthUsecase(repo HealthRepo, logger log.Logger) *HealthUsecase {
	return &HealthUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// Check checks backend healthiness.
func (uc *HealthUsecase) Check(ctx context.Context) error {
	return uc.repo.Ping(ctx)
}
