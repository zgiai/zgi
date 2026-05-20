package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

// ProviderRepository defines the interface for global provider operations
type ProviderRepository interface {
	Create(ctx context.Context, provider *model.LLMProvider) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.LLMProvider, error)
	GetByName(ctx context.Context, name string) (*model.LLMProvider, error)
	List(ctx context.Context, isActive *bool, offset, limit int) ([]*model.LLMProvider, int64, error)
	Update(ctx context.Context, provider *model.LLMProvider) error
	Delete(ctx context.Context, id uuid.UUID) error
	ExistsByName(ctx context.Context, name string) (bool, error)
}

// ProviderConfigRepository defines the interface for tenant provider config operations
type ProviderConfigRepository interface {
	Create(ctx context.Context, config *model.ProviderConfig) error
	GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.ProviderConfig, error)
	GetByProviderID(ctx context.Context, organizationID, providerID uuid.UUID) (*model.ProviderConfig, error)
	List(ctx context.Context, organizationID uuid.UUID, isEnabled *bool, offset, limit int) ([]*model.ProviderConfig, int64, error)
	Update(ctx context.Context, config *model.ProviderConfig) error
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
	Upsert(ctx context.Context, config *model.ProviderConfig) error
}

// CustomProviderRepository defines the interface for tenant custom provider operations
type CustomProviderRepository interface {
	Create(ctx context.Context, provider *model.CustomProvider) error
	GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomProvider, error)
	GetByProvider(ctx context.Context, organizationID uuid.UUID, provider string) (*model.CustomProvider, error)
	List(ctx context.Context, organizationID uuid.UUID, isActive *bool, offset, limit int) ([]*model.CustomProvider, int64, error)
	Update(ctx context.Context, provider *model.CustomProvider) error
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
	ExistsByProvider(ctx context.Context, organizationID uuid.UUID, provider string) (bool, error)
}
