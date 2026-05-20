package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/credential/dto"
	"github.com/zgiai/ginext/internal/modules/llm/credential/model"
)

// TenantCredentialService defines the interface for tenant credential operations
type TenantCredentialService interface {
	Create(ctx context.Context, organizationID uuid.UUID, req *dto.CreateTenantCredentialRequest) (*model.TenantCredential, error)
	GetOrCreateByAPIKey(ctx context.Context, organizationID uuid.UUID, req *dto.CreateTenantCredentialRequest) (*model.TenantCredential, bool, error)
	GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.TenantCredential, error)
	List(ctx context.Context, organizationID uuid.UUID, req *dto.ListCredentialRequest) ([]*model.TenantCredential, int64, error)
	Update(ctx context.Context, organizationID, id uuid.UUID, req *dto.UpdateTenantCredentialRequest) (*model.TenantCredential, error)
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
	GetDecryptedAPIKey(ctx context.Context, organizationID, id uuid.UUID) (string, error)
	TestCredential(ctx context.Context, organizationID, id uuid.UUID, model string, apiBaseURL string) (*dto.TestCredentialResult, error)
}
