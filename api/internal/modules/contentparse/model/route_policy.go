package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoutePolicy struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Scope         string         `gorm:"type:varchar(32);not null;index:idx_content_parse_policy_scope,priority:1" json:"scope"`
	WorkspaceID   *uuid.UUID     `gorm:"type:uuid;column:workspace_id;index:idx_content_parse_policy_scope,priority:2" json:"workspace_id,omitempty"`
	PolicyKey     string         `gorm:"type:varchar(64);not null;index:idx_content_parse_policy_scope,priority:3" json:"policy_key"`
	DisplayName   string         `gorm:"type:varchar(128);not null" json:"display_name"`
	Enabled       bool           `gorm:"not null;default:true" json:"enabled"`
	AllowRemote   bool           `gorm:"not null;default:true" json:"allow_remote"`
	AllowFallback bool           `gorm:"not null;default:true" json:"allow_fallback"`
	Metadata      map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"metadata,omitempty"`
	CreatedBy     *uuid.UUID     `gorm:"type:uuid;column:created_by" json:"created_by,omitempty"`
	UpdatedBy     *uuid.UUID     `gorm:"type:uuid;column:updated_by" json:"updated_by,omitempty"`
	CreatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (RoutePolicy) TableName() string {
	return "content_parse_route_policies"
}

func (m *RoutePolicy) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
