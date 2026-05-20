package dto

// ListCredentialRequest represents the request to list credentials
type ListCredentialRequest struct {
	Provider string `form:"provider"`
	IsActive *bool  `form:"is_active"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
}

// CreateTenantCredentialRequest represents the request to create a tenant credential
type CreateTenantCredentialRequest struct {
	Name            string `json:"name" binding:"required"`
	ChannelProvider string `json:"channel_provider" binding:"required"`
	APIKey          string `json:"api_key" binding:"required"`
	APIBaseURL      string `json:"api_base_url"`
}

// UpdateTenantCredentialRequest represents the request to update a tenant credential
type UpdateTenantCredentialRequest struct {
	Name            *string `json:"name"`
	ChannelProvider *string `json:"channel_provider"`
	APIKey          *string `json:"api_key"`
	APIBaseURL      *string `json:"api_base_url"`
	IsActive        *bool   `json:"is_active"`
}

// TestCredentialRequest represents the request to test a credential
type TestCredentialRequest struct {
	Model string `json:"model" binding:"required"`
}

// TestCredentialResult represents the result of testing a credential
type TestCredentialResult struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	ResponseTimeMs int64  `json:"response_time_ms"`
	Model          string `json:"model,omitempty"`
	Response       string `json:"response,omitempty"`
}
