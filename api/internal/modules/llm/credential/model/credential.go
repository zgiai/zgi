package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TenantCredential represents encrypted API credentials owned by tenants.

type TenantCredential struct {
	ID               uuid.UUID              `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID   uuid.UUID              `gorm:"type:uuid;not null;index:idx_organization_cred_organization;column:organization_id" json:"organization_id"`
	Name             string                 `gorm:"type:varchar(100);not null" json:"name"`
	ChannelProvider  string                 `gorm:"column:provider;type:varchar(50);not null;index:idx_organization_cred_provider" json:"channel_provider"`
	APIKeyCiphertext string                 `gorm:"type:text;not null" json:"-"`
	APIKeyHash       string                 `gorm:"type:varchar(64)" json:"-"`
	APIBaseURL       string                 `gorm:"type:varchar(500)" json:"api_base_url"`
	IsActive         bool                   `gorm:"default:true;index:idx_organization_cred_active" json:"is_active"`
	LastUsedAt       *time.Time             `json:"last_used_at,omitempty"`
	ExpiresAt        *time.Time             `json:"expires_at,omitempty"`
	Metadata         map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`
	CreatedAt        time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt        time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt        gorm.DeletedAt         `gorm:"index:idx_organization_cred_deleted_at" json:"deleted_at,omitempty"`
}

func (TenantCredential) TableName() string {
	return "llm_credentials"
}

func (c *TenantCredential) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// IsExpired returns true if the credential has expired
func (c *TenantCredential) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*c.ExpiresAt)
}

// IsUsable returns true if the credential can be used
func (c *TenantCredential) IsUsable() bool {
	return c.IsActive && !c.IsExpired()
}
