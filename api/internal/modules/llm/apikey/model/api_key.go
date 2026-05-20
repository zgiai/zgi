package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TenantAPIKey represents an API key for accessing LLM services
type TenantAPIKey struct {
	ID             string `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID string `gorm:"type:uuid;not null;index;column:organization_id" json:"organization_id"`
	Key            string `gorm:"type:text;not null" json:"key"`         // Encrypted API key
	KeyHash        string `gorm:"type:varchar(64);uniqueIndex" json:"-"` // SHA-256 hash for querying (optional)
	Name           string `gorm:"type:varchar(255);not null" json:"name"`
	Status         string `gorm:"type:varchar(20);not null;default:'active'" json:"status"`

	// Internal use only
	IsInternal bool `gorm:"not null;default:false" json:"is_internal"`

	// Time fields
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	AccessedAt *time.Time     `json:"accessed_at,omitempty"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Quota management
	UsedQuota   int64  `gorm:"not null;default:0" json:"used_quota"`
	RemainQuota int64  `gorm:"not null;default:0" json:"remain_quota"`
	QuotaLimit  *int64 `json:"quota_limit,omitempty"`

	// Model limits
	ModelLimitsEnabled bool    `gorm:"not null;default:false" json:"model_limits_enabled"`
	ModelLimits        *string `gorm:"type:jsonb" json:"model_limits,omitempty"`

	// IP whitelist
	AllowIPs string `gorm:"type:text;not null;default:''" json:"allow_ips"`
}

// TableName specifies the table name for TenantAPIKey
func (TenantAPIKey) TableName() string {
	return "llm_organization_api_keys"
}

func (k *TenantAPIKey) BeforeCreate(tx *gorm.DB) error {
	if k.ID == "" {
		k.ID = uuid.NewString()
	}
	return nil
}

// IsActive checks if the API key is active
func (k *TenantAPIKey) IsActive() bool {
	if k.Status != "active" {
		return false
	}

	// Check expiration
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return false
	}

	return true
}

// HasQuota checks if the API key has remaining quota
func (k *TenantAPIKey) HasQuota() bool {
	// If quota_limit is NULL, it means unlimited quota
	if k.QuotaLimit == nil {
		return true
	}

	return k.RemainQuota > 0
}

// IsIPAllowed checks if the given IP is allowed
func (k *TenantAPIKey) IsIPAllowed(ip string) bool {
	// If allow_ips is empty, all IPs are allowed
	if k.AllowIPs == "" {
		return true
	}

	// TODO: Implement IP whitelist checking logic
	// This is a placeholder implementation
	return true
}
