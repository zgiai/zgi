package dto

import "time"

// ============ LLM API Key DTOs ============

// QuotaType represents the quota type for API key
type QuotaType string

const (
	QuotaTypeUnlimited QuotaType = "unlimited" // unlimited
	QuotaTypeCustom    QuotaType = "custom"    // custom quota
)

// LLMAPIKeyCreateRequest represents API key creation request
type LLMAPIKeyCreateRequest struct {
	// Required fields
	Name     string  `json:"name" binding:"required,min=1,max=255"` // key name
	TenantID *string `json:"tenant_id"`                             // department (tenant ID)

	// Expiration settings
	ExpiresAt *time.Time `json:"expires_at"` // expiration time, nil means never expires

	// Quota settings
	QuotaType   QuotaType `json:"quota_type" binding:"required,oneof=unlimited custom"` // quota settings: unlimited or custom
	QuotaAmount *int64    `json:"quota_amount" binding:"omitempty,min=1"`               // custom quota amount, required when quota_type is custom

	// Batch creation
	Count int `json:"count" binding:"required,min=1,max=100"` // creation count, controls how many keys to create

	// Optional fields
	AllowAllModels bool     `json:"allow_all_models"` // allow access to all models
	ModelNames     []string `json:"model_names"`      // specified model name list, from llm_tenant_model_configs where tenant_id matches
	AllowIPs       string   `json:"allow_ips"`        // IP whitelist, comma-separated
}

// LLMAPIKeyUpdateRequest represents API key update request
type LLMAPIKeyUpdateRequest struct {
	// Required fields
	Name     *string `json:"name" binding:"omitempty,min=1,max=255"` // key name
	TenantID *string `json:"tenant_id"`                              // department (tenant ID)

	// Status settings
	Status *string `json:"status" binding:"omitempty,oneof=active inactive revoked"` // API key status

	// Expiration settings
	ExpiresAt *time.Time `json:"expires_at"` // expiration time, nil means never expires

	// Quota settings
	QuotaType   *QuotaType `json:"quota_type" binding:"omitempty,oneof=unlimited custom"` // quota settings: unlimited or custom
	QuotaAmount *int64     `json:"quota_amount" binding:"omitempty,min=1"`                // custom quota amount, required when quota_type is custom

	// Optional fields
	AllowAllModels *bool    `json:"allow_all_models"` // allow access to all models
	ModelNames     []string `json:"model_names"`      // specified model name list, from llm_tenant_model_configs where tenant_id matches
	AllowIPs       *string  `json:"allow_ips"`        // IP whitelist, comma-separated
}

// LLMAPIKeyListRequest represents API key list query request
type LLMAPIKeyListRequest struct {
	Page      int        `form:"page" binding:"omitempty,min=1"`
	Limit     int        `form:"limit" binding:"omitempty,min=1,max=100"`
	Status    string     `form:"status" binding:"omitempty,oneof=active inactive revoked"`
	Search    string     `form:"search"`
	StartDate *time.Time `form:"start_date"` // filter by creation date >= start_date
	EndDate   *time.Time `form:"end_date"`   // filter by creation date <= end_date
}

// LLMAPIKeyResponse represents API key response
type LLMAPIKeyResponse struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenant_id"`
	TenantName         string     `json:"tenant_name"`
	Key                string     `json:"key"`        // Decrypted key
	KeyMasked          string     `json:"key_masked"` // Obfuscated key (e.g., "sk-12**************ef")
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	AccessedAt         *time.Time `json:"accessed_at"`
	ExpiresAt          *time.Time `json:"expires_at"`
	UsedQuota          int64      `json:"used_quota"`
	RemainQuota        int64      `json:"remain_quota"`
	QuotaLimit         *int64     `json:"quota_limit"`
	ModelLimitsEnabled bool       `json:"model_limits_enabled"`
	ModelLimits        []string   `json:"model_limits"`
	AllowIPs           string     `json:"allow_ips"`
}

// LLMAPIKeyListResponse represents paginated API key list
type LLMAPIKeyListResponse struct {
	Items      []LLMAPIKeyResponse `json:"items"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	Limit      int                 `json:"limit"`
	TotalPages int                 `json:"total_pages"`
}

// LLMAPIKeyCreateResponse represents API key creation response
type LLMAPIKeyCreateResponse struct {
	Keys    []LLMAPIKeyResponse `json:"keys"`    // List of created API keys
	Count   int                 `json:"count"`   // Number of keys created
	Message string              `json:"message"` // Success message
}

// LLMAPIKeyDeleteResponse represents API key deletion response
type LLMAPIKeyDeleteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// LLMAPIKeyValidateRequest represents API key validation request
type LLMAPIKeyValidateRequest struct {
	Key string `json:"key" binding:"required"`
}

// LLMAPIKeyValidateResponse represents API key validation response
type LLMAPIKeyValidateResponse struct {
	Valid   bool               `json:"valid"`
	Message string             `json:"message,omitempty"`
	APIKey  *LLMAPIKeyResponse `json:"api_key,omitempty"`
}
