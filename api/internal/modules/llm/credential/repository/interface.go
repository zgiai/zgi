package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/credential/model"
)

// TenantCredentialRepository defines the interface for tenant credential operations
type TenantCredentialRepository interface {
	Create(ctx context.Context, credential *model.TenantCredential) error
	GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.TenantCredential, error)
	GetByHash(ctx context.Context, organizationID uuid.UUID, hash string) (*model.TenantCredential, error)
	GetBySignature(ctx context.Context, organizationID uuid.UUID, hash, channelProvider, apiBaseURL string) (*model.TenantCredential, error)
	List(ctx context.Context, organizationID uuid.UUID, provider string, isActive *bool, offset, limit int) ([]*model.TenantCredential, int64, error)
	Update(ctx context.Context, credential *model.TenantCredential) error
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
	ExistsByHash(ctx context.Context, organizationID uuid.UUID, hash string) (bool, error)
}
