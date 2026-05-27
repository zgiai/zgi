package agents

import (
	"time"

	"github.com/google/uuid"
)

type AgentPublishedVersion struct {
	ID             uuid.UUID              `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	AgentID        uuid.UUID              `gorm:"type:uuid;not null;column:agent_id" json:"agent_id"`
	WorkspaceID    uuid.UUID              `gorm:"type:uuid;not null;column:workspace_id" json:"workspace_id"`
	Version        string                 `gorm:"type:varchar(255);not null" json:"version"`
	VersionUUID    uuid.UUID              `gorm:"type:uuid;not null;column:version_uuid" json:"version_uuid"`
	ConfigSnapshot map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:config_snapshot" json:"config_snapshot"`
	Description    string                 `gorm:"type:text;not null;default:''" json:"description"`
	CreatedBy      *uuid.UUID             `gorm:"type:uuid;column:created_by" json:"created_by"`
	CreatedAt      time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	DeletedAt      *time.Time             `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
}

func (AgentPublishedVersion) TableName() string {
	return "agent_published_versions"
}
