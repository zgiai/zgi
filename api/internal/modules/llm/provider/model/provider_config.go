package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProviderConfig represents organization-specific configuration for a global provider
type ProviderConfig struct {
	ID                uuid.UUID              `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID    uuid.UUID              `gorm:"type:uuid;not null;index;column:organization_id" json:"organization_id"`
	ProviderID        uuid.UUID              `gorm:"type:uuid;not null" json:"provider_id"`
	IsEnabled         bool                   `gorm:"default:true;index" json:"is_enabled"`
	CustomDisplayName string                 `gorm:"type:varchar(100)" json:"custom_display_name,omitempty"`
	CustomAPIBaseURL  string                 `gorm:"type:varchar(255)" json:"custom_api_base_url,omitempty"`
	CustomLogoURL     string                 `gorm:"type:varchar(255)" json:"custom_logo_url,omitempty"`
	SortOrder         int                    `gorm:"default:0" json:"sort_order"`
	Metadata          map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`
	CreatedAt         time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt         time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt         gorm.DeletedAt         `gorm:"index" json:"deleted_at,omitempty"`

	// Relations
	Provider *LLMProvider `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
}

func (ProviderConfig) TableName() string {
	return "llm_provider_configs"
}

func (c *ProviderConfig) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// GetEffectiveDisplayName returns the custom display name or falls back to provider's display name
func (c *ProviderConfig) GetEffectiveDisplayName() string {
	if c.CustomDisplayName != "" {
		return c.CustomDisplayName
	}
	if c.Provider != nil {
		return c.Provider.ProviderName
	}
	return ""
}

// GetEffectiveAPIBaseURL returns the custom API base URL or falls back to provider's URL
func (c *ProviderConfig) GetEffectiveAPIBaseURL() string {
	if c.CustomAPIBaseURL != "" {
		return c.CustomAPIBaseURL
	}
	if c.Provider != nil {
		return c.Provider.APIBaseURL
	}
	return ""
}
