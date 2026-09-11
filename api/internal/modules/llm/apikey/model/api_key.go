package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TenantAPIKey represents an API key for accessing LLM services
type TenantAPIKey struct {
	ID                   string     `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID       string     `gorm:"type:uuid;not null;index;column:organization_id" json:"organization_id"`
	Key                  string     `gorm:"type:text" json:"key,omitempty"`        // Legacy encrypted API key. New keys are hash-only.
	KeyHash              string     `gorm:"type:varchar(64);uniqueIndex" json:"-"` // SHA-256 hash for querying (optional)
	Name                 string     `gorm:"type:varchar(255);not null" json:"name"`
	Status               string     `gorm:"type:varchar(20);not null;default:'active'" json:"status"`
	Environment          string     `gorm:"type:varchar(20);not null;default:'development'" json:"environment"`
	WorkspaceID          *string    `gorm:"type:uuid;index;column:workspace_id" json:"workspace_id,omitempty"`
	PrincipalType        *string    `gorm:"type:varchar(32);column:principal_type" json:"principal_type,omitempty"`
	PrincipalID          *string    `gorm:"type:varchar(255);index;column:principal_id" json:"principal_id,omitempty"`
	AccessGrantID        *string    `gorm:"type:uuid;index;column:access_grant_id" json:"access_grant_id,omitempty"`
	CreatedByID          *string    `gorm:"type:uuid;column:created_by_account_id" json:"created_by_account_id,omitempty"`
	KeyPrefix            string     `gorm:"type:varchar(16);not null;default:'';column:key_prefix" json:"key_prefix,omitempty"`
	KeySuffix            string     `gorm:"type:varchar(8);not null;default:'';column:key_suffix" json:"key_suffix,omitempty"`
	SecretVersion        int        `gorm:"not null;default:1;column:secret_format_version" json:"secret_format_version"`
	RevokedAt            *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
	RevokedByID          *string    `gorm:"type:uuid;column:revoked_by_account_id" json:"revoked_by_account_id,omitempty"`
	RevokedReason        *string    `gorm:"type:varchar(500);column:revoked_reason" json:"revoked_reason,omitempty"`
	RotatedFromID        *string    `gorm:"type:uuid;column:rotated_from_key_id" json:"rotated_from_key_id,omitempty"`
	AuthorizationVersion int64      `gorm:"not null;default:1;column:authorization_version" json:"authorization_version"`

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

// IsIPAllowed is currently a placeholder and does not enforce allow_ips.
func (k *TenantAPIKey) IsIPAllowed(ip string) bool {
	// No configured value means there is nothing to evaluate in this placeholder.
	if k.AllowIPs == "" {
		return true
	}

	// TODO: Enforce allow_ips only after gateway IP trust is defined.
	return true
}
