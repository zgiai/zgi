package dto

import (
	"time"

	"github.com/google/uuid"
)

// TenantProviderListRequest represents the request to list tenant providers with pagination
type TenantProviderListRequest struct {
	Page      int   `form:"page" binding:"omitempty,min=1"`
	Limit     int   `form:"limit" binding:"omitempty,min=1,max=100"`
	IsEnabled *bool `form:"is_enabled"` // Filter by enabled status (true=enabled only, false=disabled only, nil=all)
}

// TenantProviderListResponse represents a provider with tenant-specific enabled status
type TenantProviderListResponse struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	DisplayName      string    `json:"display_name"`
	APIBaseURL       string    `json:"api_base_url,omitempty"`
	APIDocsURL string    `json:"documentation_url,omitempty"`
	LogoURL          string    `json:"logo_url,omitempty"`
	IsEnabled        bool      `json:"is_enabled"` // Whether this provider is enabled for the current tenant
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TenantProviderPaginatedResponse represents paginated tenant provider list response
type TenantProviderPaginatedResponse struct {
	Items   []TenantProviderListResponse `json:"items"`
	Total   int64                        `json:"total"`
	Page    int                          `json:"page"`
	Limit   int                          `json:"limit"`
	HasMore bool                         `json:"has_more"`
}

// TenantProviderToggleRequest represents the request to enable/disable a provider for a tenant
type TenantProviderToggleRequest struct {
	Provider  string `json:"provider" binding:"required"`
	IsEnabled bool   `json:"is_enabled"`
}

// TenantProviderDetailResponse represents detailed provider information
type TenantProviderDetailResponse struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	DisplayName      string    `json:"display_name"`
	APIBaseURL       string    `json:"api_base_url,omitempty"`
	APIDocsURL string    `json:"documentation_url,omitempty"`
	IsEnabled        bool      `json:"is_enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Description      string    `json:"description"`
}

// TenantModelToggleRequest represents the request to enable/disable a model for a tenant
type TenantModelToggleRequest struct {
	ModelName string `json:"model_name" binding:"required"`
	IsEnabled bool   `json:"is_enabled"`
}
