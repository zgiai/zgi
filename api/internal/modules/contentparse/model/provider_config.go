package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProviderConfig struct {
	ID                    uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Scope                 string         `gorm:"type:varchar(32);not null;index:idx_content_parse_provider_scope,priority:1" json:"scope"`
	OrganizationID        *uuid.UUID     `gorm:"type:uuid;column:organization_id;index:idx_content_parse_provider_scope,priority:2" json:"organization_id,omitempty"`
	WorkspaceID           *uuid.UUID     `gorm:"type:uuid;column:workspace_id;index:idx_content_parse_provider_scope,priority:3" json:"workspace_id,omitempty"`
	ProviderKey           string         `gorm:"type:varchar(64);not null;index:idx_content_parse_provider_scope,priority:4" json:"provider_key"`
	ProviderType          string         `gorm:"type:varchar(32);not null" json:"provider_type"`
	DisplayName           string         `gorm:"type:varchar(128);not null" json:"display_name"`
	Enabled               bool           `gorm:"not null;default:true" json:"enabled"`
	Priority              int            `gorm:"not null;default:100" json:"priority"`
	AdapterName           string         `gorm:"type:varchar(64);not null" json:"adapter_name"`
	EngineName            string         `gorm:"type:varchar(64)" json:"engine_name,omitempty"`
	BaseURL               string         `gorm:"type:text" json:"base_url,omitempty"`
	CredentialsCiphertext map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"credentials_ciphertext,omitempty"`
	TimeoutSec            int            `gorm:"not null;default:180" json:"timeout_sec"`
	SupportsFileTypes     []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"supports_file_types,omitempty"`
	SupportsProfiles      []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"supports_profiles,omitempty"`
	CostLevel             string         `gorm:"type:varchar(32)" json:"cost_level,omitempty"`
	PrivacyLevel          string         `gorm:"type:varchar(32)" json:"privacy_level,omitempty"`
	Metadata              map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"metadata,omitempty"`
	CreatedBy             *uuid.UUID     `gorm:"type:uuid;column:created_by" json:"created_by,omitempty"`
	UpdatedBy             *uuid.UUID     `gorm:"type:uuid;column:updated_by" json:"updated_by,omitempty"`
	CreatedAt             time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt             time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (ProviderConfig) TableName() string {
	return "content_parse_provider_configs"
}

func (m *ProviderConfig) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
