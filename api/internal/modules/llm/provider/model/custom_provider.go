package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CustomProvider represents a custom provider created by an organization
// Field naming aligned with global Provider model (ModelMeta standard)
type CustomProvider struct {
	ID             uuid.UUID              `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID uuid.UUID              `gorm:"type:uuid;not null;index;column:organization_id" json:"organization_id"`
	Provider       string                 `gorm:"type:varchar(50);not null;column:provider" json:"provider"`
	ProviderName   string                 `gorm:"type:varchar(100);not null;column:provider_name" json:"provider_name"`
	APIBaseURL     string                 `gorm:"type:varchar(255)" json:"api_base_url"`
	LogoURL        string                 `gorm:"type:varchar(255)" json:"logo_url,omitempty"`
	APIDocsURL     string                 `gorm:"column:documentation_url" json:"documentation_url,omitempty"`
	Description    string                 `gorm:"type:text" json:"description,omitempty"`
	IsActive       bool                   `gorm:"default:true;index" json:"is_active"`
	SortOrder      int                    `gorm:"default:0" json:"sort_order"`
	Metadata       map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`
	CreatedAt      time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt      gorm.DeletedAt         `gorm:"index" json:"deleted_at,omitempty"`
}

func (CustomProvider) TableName() string {
	return "llm_custom_providers"
}

func (p *CustomProvider) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
