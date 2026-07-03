package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
)

type ProviderAdminService interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.ProviderConfig, error)
	ListByScope(ctx context.Context, scope string, organizationID, workspaceID *uuid.UUID) ([]*model.ProviderConfig, error)
	Create(ctx context.Context, item *model.ProviderConfig) error
	UpsertByScopeAndKey(ctx context.Context, item *model.ProviderConfig) error
	Update(ctx context.Context, item *model.ProviderConfig) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type providerAdminService struct {
	repo repository.ProviderConfigRepository
}

func NewProviderAdminService(repo repository.ProviderConfigRepository) ProviderAdminService {
	return &providerAdminService{repo: repo}
}

func (s *providerAdminService) GetByID(ctx context.Context, id uuid.UUID) (*model.ProviderConfig, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *providerAdminService) ListByScope(ctx context.Context, scope string, organizationID, workspaceID *uuid.UUID) ([]*model.ProviderConfig, error) {
	return s.repo.ListByScope(ctx, scope, organizationID, workspaceID)
}

func (s *providerAdminService) Create(ctx context.Context, item *model.ProviderConfig) error {
	return s.repo.Create(ctx, item)
}

func (s *providerAdminService) UpsertByScopeAndKey(ctx context.Context, item *model.ProviderConfig) error {
	return s.repo.UpsertByScopeAndKey(ctx, item)
}

func (s *providerAdminService) Update(ctx context.Context, item *model.ProviderConfig) error {
	return s.repo.Update(ctx, item)
}

func (s *providerAdminService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
