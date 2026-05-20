package interfaces

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

type LLMProviderService interface {
	// CreateProvider creates a new LLM provider
	CreateProvider(ctx context.Context, req *dto.LLMProviderCreateRequest) (*providermodel.LLMProvider, error)
	// UpdateProvider updates an existing LLM provider
	UpdateProvider(ctx context.Context, id uuid.UUID, req *dto.LLMProviderUpdateRequest) (*providermodel.LLMProvider, error)
	// DeleteProvider deletes an LLM provider by ID
	DeleteProvider(ctx context.Context, id uuid.UUID) error
	// GetProviderByID retrieves an LLM provider by ID
	GetProviderByID(ctx context.Context, id uuid.UUID) (*providermodel.LLMProvider, error)
	// GetProviderByName retrieves an LLM provider by name (from cache, without API key)
	GetProviderByName(ctx context.Context, name string) (*providermodel.LLMProvider, error)
	// GetProviderByNameWithAPIKey retrieves an LLM provider by name with API key (bypasses cache)
	GetProviderByNameWithAPIKey(ctx context.Context, name string) (*providermodel.LLMProvider, error)
	// ListProviders retrieves a paginated list of LLM providers
	ListProviders(ctx context.Context, req *dto.LLMProviderListRequest) ([]*providermodel.LLMProvider, int64, error)
	// GetProviderModels retrieves all models associated with a provider
	GetProviderModels(ctx context.Context, providerID uuid.UUID) ([]*llmmodel.LLMModel, error)

	// SyncModels fetches models from the provider's API and updates the database
	SyncModels(ctx context.Context, id uuid.UUID) (*dto.LLMProviderSyncResponse, error)
}
