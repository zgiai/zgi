package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"github.com/zgiai/ginext/internal/modules/contentparse/repository"
)

type HealthService interface {
	Record(ctx context.Context, item *model.ProviderHealthCheck) error
	ListByProviderConfigID(ctx context.Context, providerConfigID uuid.UUID, limit int) ([]*model.ProviderHealthCheck, error)
	GetLatestByProviderConfigID(ctx context.Context, providerConfigID uuid.UUID) (*model.ProviderHealthCheck, error)
}

type healthService struct {
	repo repository.ProviderHealthRepository
}

func NewHealthService(repo repository.ProviderHealthRepository) HealthService {
	return &healthService{repo: repo}
}

func (s *healthService) Record(ctx context.Context, item *model.ProviderHealthCheck) error {
	return s.repo.Create(ctx, item)
}

func (s *healthService) ListByProviderConfigID(ctx context.Context, providerConfigID uuid.UUID, limit int) ([]*model.ProviderHealthCheck, error) {
	return s.repo.ListByProviderConfigID(ctx, providerConfigID, limit)
}

func (s *healthService) GetLatestByProviderConfigID(ctx context.Context, providerConfigID uuid.UUID) (*model.ProviderHealthCheck, error) {
	return s.repo.GetLatestByProviderConfigID(ctx, providerConfigID)
}
