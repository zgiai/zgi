package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
)

// ModelRepository defines the interface for global model operations
type ModelRepository interface {
	Create(ctx context.Context, m *model.LLMModel) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.LLMModel, error)
	GetByName(ctx context.Context, name string) (*model.LLMModel, error)
	ListByNames(ctx context.Context, names []string) ([]*model.LLMModel, error)
	ListAvailableByNames(ctx context.Context, names []string, provider string, useCase string) ([]*model.LLMModel, error)
	ListAvailableFiltered(ctx context.Context, provider string, useCase string) ([]*model.LLMModel, error)
	GetByProviderAndName(ctx context.Context, provider string, name string) (*model.LLMModel, error)
	List(ctx context.Context, providerID *uuid.UUID, provider string, useCase string, isActive *bool, offset, limit int) ([]*model.LLMModel, int64, error)
	Update(ctx context.Context, m *model.LLMModel) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByProvider(ctx context.Context, providerID string) ([]*model.LLMModel, error)
}

// ModelConfigRepository defines the interface for model config operations
type ModelConfigRepository interface {
	Create(ctx context.Context, config *model.ModelConfig) error
	GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.ModelConfig, error)
	GetByModelID(ctx context.Context, organizationID, modelID uuid.UUID) (*model.ModelConfig, error)
	List(ctx context.Context, organizationID uuid.UUID, isEnabled *bool, offset, limit int) ([]*model.ModelConfig, int64, error)
	ListAvailableConfigs(ctx context.Context, organizationID uuid.UUID) ([]*model.ModelConfig, error)
	Update(ctx context.Context, config *model.ModelConfig) error
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
	Upsert(ctx context.Context, config *model.ModelConfig) error
	BatchCreate(ctx context.Context, configs []*model.ModelConfig) error
}

// CustomModelRepository defines the interface for custom model operations
type CustomModelRepository interface {
	Create(ctx context.Context, m *model.CustomModel) error
	GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomModel, error)
	GetByProviderAndName(ctx context.Context, organizationID, providerID uuid.UUID, name string) (*model.CustomModel, error)
	GetByProviderAndModel(ctx context.Context, organizationID uuid.UUID, provider string, name string) (*model.CustomModel, error)
	ListByNames(ctx context.Context, organizationID uuid.UUID, names []string, isActive *bool) ([]*model.CustomModel, error)
	List(ctx context.Context, organizationID uuid.UUID, providerID *uuid.UUID, provider string, useCase string, isActive *bool, offset, limit int) ([]*model.CustomModel, int64, error)
	Update(ctx context.Context, m *model.CustomModel) error
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
	ListByProvider(ctx context.Context, organizationID, providerID uuid.UUID) ([]*model.CustomModel, error)
}
