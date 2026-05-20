package service

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/llm/apikey/dto"
)

// APIKeyService defines the interface for API key operations
type APIKeyService interface {
	// CreateAPIKey creates a new API key
	CreateAPIKey(ctx context.Context, req *dto.CreateAPIKeyRequest) (*dto.CreateAPIKeyResponse, error)

	// GetAPIKey gets an API key by ID
	GetAPIKey(ctx context.Context, id string, organizationIDs []string) (*dto.APIKeyResponse, error)

	// ListAPIKeys lists API keys with filters and pagination
	ListAPIKeys(ctx context.Context, req *dto.ListAPIKeyRequest) (*dto.ListAPIKeyResponse, error)

	// UpdateAPIKey updates an API key
	UpdateAPIKey(ctx context.Context, id string, organizationIDs []string, req *dto.UpdateAPIKeyRequest) (*dto.APIKeyResponse, error)

	// DeleteAPIKey deletes an API key
	DeleteAPIKey(ctx context.Context, id string, organizationIDs []string) (*dto.DeleteAPIKeyResponse, error)

	// ValidateAPIKey validates an API key
	ValidateAPIKey(ctx context.Context, key string) (*dto.ValidateAPIKeyResponse, error)

	// UpdateAccessedAt updates the last accessed time
	UpdateAccessedAt(ctx context.Context, id string) error
}
