package dto

import "github.com/google/uuid"

// CreateProviderRequest is the request for creating a global provider
type CreateProviderRequest struct {
	Name         string `json:"provider" binding:"required"`
	ProviderName string `json:"provider_name" binding:"required"`
	APIBaseURL   string `json:"api_base_url"`
	LogoURL      string `json:"logo_url"`
	APIDocsURL   string `json:"documentation_url"`
	Description  string `json:"description"`
	ProviderType string `json:"provider_type"`

	// ModelMeta aligned fields
	Website     string `json:"website"`
	PricingURL  string `json:"pricing_url"`
	Tagline     string `json:"tagline"`
	CountryCode string `json:"country_code"`
	FoundedYear int    `json:"founded_year"`
}

// UpdateProviderRequest is the request for updating a global provider
type UpdateProviderRequest struct {
	ProviderName *string `json:"provider_name"`
	APIBaseURL   *string `json:"api_base_url"`
	LogoURL      *string `json:"logo_url"`
	APIDocsURL   *string `json:"documentation_url"`
	Description  *string `json:"description"`
	IsActive     *bool   `json:"is_active"`
	SortOrder    *int    `json:"sort_order"`

	// ModelMeta aligned fields
	Website     *string `json:"website"`
	PricingURL  *string `json:"pricing_url"`
	Tagline     *string `json:"tagline"`
	CountryCode *string `json:"country_code"`
	FoundedYear *int    `json:"founded_year"`
}

// ListProviderRequest is the request for listing providers
type ListProviderRequest struct {
	IsActive *bool `form:"is_active"`
	Page     int   `form:"page,default=1"`
	PageSize int   `form:"page_size,default=1000"`
}

// ConfigureProviderRequest is the request for configuring a tenant's provider
type ConfigureProviderRequest struct {
	ProviderID        uuid.UUID `json:"provider_id" binding:"required"`
	IsEnabled         *bool     `json:"is_enabled"`
	CustomDisplayName string    `json:"custom_display_name"`
	CustomAPIBaseURL  string    `json:"custom_api_base_url"`
	CustomLogoURL     string    `json:"custom_logo_url"`
	SortOrder         *int      `json:"sort_order"`
}

// CreateCustomProviderRequest is the request for creating a tenant's custom provider
type CreateCustomProviderRequest struct {
	Provider     string `json:"provider" binding:"required"`
	ProviderName string `json:"provider_name" binding:"required"`
	APIBaseURL   string `json:"api_base_url"`
	LogoURL      string `json:"logo_url"`
	APIDocsURL   string `json:"documentation_url"`
	Description  string `json:"description"`
}

// UpdateCustomProviderRequest is the request for updating a tenant's custom provider
type UpdateCustomProviderRequest struct {
	ProviderName *string `json:"provider_name"`
	APIBaseURL   *string `json:"api_base_url"`
	LogoURL      *string `json:"logo_url"`
	APIDocsURL   *string `json:"documentation_url"`
	Description  *string `json:"description"`
	IsActive     *bool   `json:"is_active"`
	SortOrder    *int    `json:"sort_order"`
}

// ToggleProviderRequest is the request for toggling a provider's enabled status
type ToggleProviderRequest struct {
	Provider  string `json:"provider" binding:"required"`
	IsEnabled bool   `json:"is_enabled"`
}

// ToggleModelRequest is the request for toggling a model's enabled status under a provider
type ToggleModelRequest struct {
	Model     string `json:"model"`
	ModelName string `json:"model_name"` // Legacy field name for backward compatibility
	IsEnabled bool   `json:"is_enabled"`
}

// ProviderDetailResponse represents detailed provider information
// Note: Models should be fetched separately via model APIs
type ProviderDetailResponse struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"provider_name"`
	LogoURL     string `json:"logo_url"`
	IsEnabled   bool   `json:"is_enabled"`
}

// TenantProviderListResponse is the paginated response for tenant providers
type TenantProviderListResponse struct {
	Items   interface{} `json:"items"`
	Total   int64       `json:"total"`
	Page    int         `json:"page"`
	Limit   int         `json:"limit"`
	HasMore bool        `json:"has_more"`
}
