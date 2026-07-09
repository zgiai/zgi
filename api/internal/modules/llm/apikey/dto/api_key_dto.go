package dto

import "time"

// QuotaType defines the type of quota for API keys
type QuotaType string

const (
	QuotaTypeUnlimited QuotaType = "unlimited"
	QuotaTypeCustom    QuotaType = "custom"
)

// CreateAPIKeyRequest represents a request to create API keys
type CreateAPIKeyRequest struct {
	OrganizationID *string    `json:"organization_id,omitempty"`
	Name           string     `json:"name" binding:"required"`
	Count          int        `json:"count"`
	QuotaType      QuotaType  `json:"quota_type"`
	QuotaAmount    *int64     `json:"quota_amount,omitempty"`
	AllowAllModels *bool      `json:"allow_all_models,omitempty"`
	ModelNames     []string   `json:"model_names,omitempty"`
	AllowIPs       string     `json:"allow_ips,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// APIKeyResponse represents an API key response
type APIKeyResponse struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organization_id"`
	OrganizationName   string     `json:"organization_name,omitempty"`
	Key                string     `json:"key,omitempty"`
	KeyMasked          string     `json:"key_masked"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	AccessedAt         *time.Time `json:"accessed_at,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	UsedQuota          int64      `json:"used_quota"`
	RemainQuota        int64      `json:"remain_quota"`
	QuotaLimit         *int64     `json:"quota_limit,omitempty"`
	ModelLimitsEnabled bool       `json:"model_limits_enabled"`
	ModelLimits        []string   `json:"model_limits,omitempty"`
	AllowIPs           string     `json:"allow_ips,omitempty"`
}

// CreateAPIKeyResponse represents the response for creating API keys
type CreateAPIKeyResponse struct {
	Keys    []APIKeyResponse `json:"keys"`
	Count   int              `json:"count"`
	Message string           `json:"message"`
}

// ListAPIKeyRequest represents a request to list API keys
type ListAPIKeyRequest struct {
	OrganizationID  *string  `form:"organization_id"`
	OrganizationIDs []string `form:"-"` // Internal use: filter by multiple tenant IDs (for group-level queries)
	Page            int      `form:"page,default=1"`
	Limit           int      `form:"limit,default=20"`
	Status          string   `form:"status"`
	Search          string   `form:"search"`
	IsInternal      *bool    `form:"is_internal"`
}

// ListAPIKeyResponse represents the response for listing API keys
type ListAPIKeyResponse struct {
	Items      []APIKeyResponse `json:"items"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	Limit      int              `json:"limit"`
	TotalPages int              `json:"total_pages"`
}

// UpdateAPIKeyRequest represents a request to update an API key
type UpdateAPIKeyRequest struct {
	Name               *string    `json:"name,omitempty"`
	Status             *string    `json:"status,omitempty"`
	QuotaLimit         *int64     `json:"quota_limit,omitempty"`
	RemainQuota        *int64     `json:"remain_quota,omitempty"`
	ModelLimitsEnabled *bool      `json:"model_limits_enabled,omitempty"`
	ModelLimits        []string   `json:"model_limits,omitempty"`
	AllowIPs           *string    `json:"allow_ips,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	ClearQuotaLimit    bool       `json:"-"`
	ClearExpiresAt     bool       `json:"-"`
}

// DeleteAPIKeyResponse represents the response for deleting an API key
type DeleteAPIKeyResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// ValidateAPIKeyRequest represents a request to validate an API key
type ValidateAPIKeyRequest struct {
	Key string `json:"key" binding:"required"`
}

// ValidateAPIKeyResponse represents the response for validating an API key
type ValidateAPIKeyResponse struct {
	Valid            bool       `json:"valid"`
	OrganizationID   string     `json:"organization_id,omitempty"`
	OrganizationName string     `json:"organization_name,omitempty"`
	KeyID            string     `json:"key_id,omitempty"`
	KeyName          string     `json:"key_name,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	Message          string     `json:"message,omitempty"`
}
