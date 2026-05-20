package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/dto"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
)

// ModelService defines the interface for model operations
type ModelService interface {
	// Global model operations (admin)
	CreateGlobal(ctx context.Context, req *dto.CreateModelRequest) (*model.LLMModel, error)
	GetGlobal(ctx context.Context, id uuid.UUID) (*model.LLMModel, error)
	ListGlobal(ctx context.Context, req *dto.ListModelRequest) ([]*model.LLMModel, int64, error)
	UpdateGlobal(ctx context.Context, id uuid.UUID, req *dto.UpdateModelRequest) (*model.LLMModel, error)
	DeleteGlobal(ctx context.Context, id uuid.UUID) error

	// Model config operations
	ConfigureModel(ctx context.Context, organizationID uuid.UUID, req *dto.ConfigureModelRequest) (*model.ModelConfig, error)
	GetModelConfig(ctx context.Context, organizationID, modelID uuid.UUID) (*model.ModelConfig, error)
	ListModelConfigs(ctx context.Context, organizationID uuid.UUID, req *dto.ListModelRequest) ([]*model.ModelConfig, int64, error)

	// Custom model operations
	CreateCustom(ctx context.Context, organizationID uuid.UUID, req *dto.CreateCustomModelRequest) (*model.CustomModel, error)
	GetCustom(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomModel, error)
	ListCustom(ctx context.Context, organizationID uuid.UUID, req *dto.ListModelRequest) ([]*model.CustomModel, int64, error)
	UpdateCustom(ctx context.Context, organizationID, id uuid.UUID, req *dto.UpdateCustomModelRequest) (*model.CustomModel, error)
	DeleteCustom(ctx context.Context, organizationID, id uuid.UUID) error

	// Aggregated operations
	ListTenantModels(ctx context.Context, organizationID uuid.UUID, useCase string, provider string) ([]*model.ModelView, error)
	GetModelParameters(ctx context.Context, organizationID uuid.UUID, provider, modelName string) (model.ConfigParameters, error)

	// Batch operations (Legacy support)
	ToggleProviderModels(ctx context.Context, organizationID uuid.UUID, provider string, isEnabled bool) error
	BatchToggleModels(ctx context.Context, organizationID uuid.UUID, modelIDs []uuid.UUID, isEnabled bool) error

	// Official models (provided by system channels)
	ListOfficialModels(ctx context.Context) ([]*model.LLMModel, error)

	// Availability testing
	CheckAvailability(ctx context.Context, organizationID uuid.UUID, modelID uuid.UUID) (*dto.ModelAvailabilityResponse, error)
	BatchCheckAvailability(ctx context.Context, organizationID uuid.UUID, req *dto.BatchModelAvailabilityRequest) (*dto.BatchModelAvailabilityResponse, error)

	// Cache management
	SetAvailableModelsService(svc AvailableModelsService)
}

// PrivateModelLookupService resolves workspace custom models for private channels and routing.
type PrivateModelLookupService interface {
	ListActiveModelsByNames(ctx context.Context, organizationID uuid.UUID, modelNames []string) ([]*model.CustomModel, error)
	ResolveActiveModels(ctx context.Context, organizationID uuid.UUID, modelNames []string) ([]*model.CustomModel, error)
	ResolveActiveModelsForProvider(ctx context.Context, organizationID uuid.UUID, provider string, modelNames []string) ([]*model.CustomModel, error)
	ResolveActiveModel(ctx context.Context, organizationID uuid.UUID, modelName string) (*model.CustomModel, error)
	ResolveActiveModelForProvider(ctx context.Context, organizationID uuid.UUID, provider string, modelName string) (*model.CustomModel, error)
	LoadActiveModelNameIndexes(ctx context.Context, organizationID uuid.UUID) ([]string, map[string]string, error)
}
