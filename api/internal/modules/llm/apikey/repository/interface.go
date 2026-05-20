package repository

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/llm/apikey/model"
)

// APIKeyRepository defines the interface for API key operations
type APIKeyRepository interface {
	// Create creates a new API key
	Create(ctx context.Context, apiKey *model.TenantAPIKey) error

	// GetByID gets an API key by ID
	GetByID(ctx context.Context, id, organizationID string) (*model.TenantAPIKey, error)

	// GetByIDInOrganizations gets an external API key by ID within allowed organizations
	GetByIDInOrganizations(ctx context.Context, id string, organizationIDs []string) (*model.TenantAPIKey, error)

	// GetByKey gets an API key by key string (deprecated, use GetByKeyHash)
	GetByKey(ctx context.Context, key string) (*model.TenantAPIKey, error)

	// GetByKeyHash gets an API key by key hash
	GetByKeyHash(ctx context.Context, keyHash string) (*model.TenantAPIKey, error)

	// List lists API keys with filters and pagination
	List(ctx context.Context, organizationID string, filters map[string]interface{}, page, limit int) ([]*model.TenantAPIKey, int64, error)

	// Update updates an API key
	Update(ctx context.Context, apiKey *model.TenantAPIKey) error

	// Delete soft deletes an API key
	Delete(ctx context.Context, id, organizationID string) error

	// UpdateAccessedAt updates the last accessed time
	UpdateAccessedAt(ctx context.Context, id string) error

	// UpdateQuota updates the quota usage
	UpdateQuota(ctx context.Context, id string, usedDelta, remainDelta int64) error

	// CountByTenant counts API keys for a tenant
	CountByTenant(ctx context.Context, organizationID string) (int64, error)
}
