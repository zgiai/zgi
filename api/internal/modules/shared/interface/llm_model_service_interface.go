package interfaces

import (
	"context"

	"github.com/zgiai/zgi/api/internal/dto"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

type LLMModelService interface {
	// CreateModel creates a new LLM model
	CreateModel(ctx context.Context, req *dto.LLMModelCreateRequest) (*llmmodel.LLMModel, error)
	// UpdateModel updates an existing LLM model
	UpdateModel(ctx context.Context, id uint, req *dto.LLMModelUpdateRequest) (*llmmodel.LLMModel, error)
	// DeleteModel deletes an LLM model by ID
	DeleteModel(ctx context.Context, id uint) error
	// GetModelByName retrieves an LLM model by name
	GetModelByName(ctx context.Context, name string) (*llmmodel.LLMModel, error)
	// GetModelsByName retrieves all LLM models by name (multiple providers may have same model name)
	GetModelsByName(ctx context.Context, name string) ([]*llmmodel.LLMModel, error)
	// GetModelByProviderAndName retrieves an LLM model by provider and name
	GetModelByProviderAndName(ctx context.Context, provider, name string) (*llmmodel.LLMModel, error)
	// ListModels retrieves a paginated list of LLM models
	ListModels(ctx context.Context, req *dto.LLMModelListRequest) ([]*llmmodel.LLMModel, int64, error)
}
