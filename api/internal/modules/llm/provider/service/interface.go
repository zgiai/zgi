package service

import (
	"context"

	"github.com/google/uuid"
	llmmodelservice "github.com/zgiai/ginext/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/ginext/internal/modules/llm/provider/dto"
	"github.com/zgiai/ginext/internal/modules/llm/provider/model"
)

// ProviderService defines the interface for provider operations
type ProviderService interface {
	// Global provider operations (admin)
	CreateGlobal(ctx context.Context, req *dto.CreateProviderRequest) (*model.LLMProvider, error)
	GetGlobal(ctx context.Context, id uuid.UUID) (*model.LLMProvider, error)
	ListGlobal(ctx context.Context, req *dto.ListProviderRequest) ([]*model.LLMProvider, int64, error)
	UpdateGlobal(ctx context.Context, id uuid.UUID, req *dto.UpdateProviderRequest) (*model.LLMProvider, error)
	DeleteGlobal(ctx context.Context, id uuid.UUID) error

	// Provider config operations
	ConfigureProvider(ctx context.Context, organizationID uuid.UUID, req *dto.ConfigureProviderRequest) (*model.ProviderConfig, error)
	GetProviderConfig(ctx context.Context, organizationID, providerID uuid.UUID) (*model.ProviderConfig, error)
	ListProviderConfigs(ctx context.Context, organizationID uuid.UUID, req *dto.ListProviderRequest) ([]*model.ProviderConfig, int64, error)

	// Custom provider operations
	CreateCustom(ctx context.Context, organizationID uuid.UUID, req *dto.CreateCustomProviderRequest) (*model.CustomProvider, error)
	GetCustom(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomProvider, error)
	ListCustom(ctx context.Context, organizationID uuid.UUID, req *dto.ListProviderRequest) ([]*model.CustomProvider, int64, error)
	UpdateCustom(ctx context.Context, organizationID, id uuid.UUID, req *dto.UpdateCustomProviderRequest) (*model.CustomProvider, error)
	DeleteCustom(ctx context.Context, organizationID, id uuid.UUID) error

	// Aggregated operations
	ListTenantProviders(ctx context.Context, organizationID uuid.UUID) ([]*model.ProviderView, error)
	GetTenantProvider(ctx context.Context, organizationID uuid.UUID, providerIdentifier string) (*model.ProviderView, error)

	// Toggle operations
	ToggleProvider(ctx context.Context, organizationID uuid.UUID, provider string, isEnabled bool) error
	GetProviderDetail(ctx context.Context, organizationID uuid.UUID, provider string) (*dto.ProviderDetailResponse, error)
	ToggleModel(ctx context.Context, organizationID uuid.UUID, provider string, modelName string, isEnabled bool) error
	SetAvailableModelsService(svc llmmodelservice.AvailableModelsService)
}
