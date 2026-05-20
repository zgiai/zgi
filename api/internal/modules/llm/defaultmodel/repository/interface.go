package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/model"
)

type DefaultModelRepository interface {
	ListByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*model.DefaultModel, error)
	GetByOrganizationAndUseCase(ctx context.Context, organizationID uuid.UUID, useCase string) (*model.DefaultModel, error)
	Upsert(ctx context.Context, item *model.DefaultModel) error
	DeleteByOrganizationAndUseCase(ctx context.Context, organizationID uuid.UUID, useCase string) error
}
