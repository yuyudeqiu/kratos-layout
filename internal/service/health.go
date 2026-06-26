package service

import (
	"context"

	"github.com/go-kratos/kratos-layout/internal/biz"
)

// HealthService handles service health endpoints.
type HealthService struct {
	uc *biz.HealthUsecase
}

// NewHealthService new a health service.
func NewHealthService(uc *biz.HealthUsecase) *HealthService {
	return &HealthService{uc: uc}
}

// Check checks liveness/readiness of dependencies.
func (s *HealthService) Check(ctx context.Context) error {
	return s.uc.Check(ctx)
}
