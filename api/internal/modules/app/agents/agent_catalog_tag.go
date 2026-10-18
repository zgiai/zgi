package agents

import (
	"time"

	"github.com/google/uuid"
)

type AgentCatalogTag struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;not null;column:organization_id"`
	Name           string     `gorm:"type:varchar(64);not null"`
	NormalizedName string     `gorm:"type:varchar(64);not null;column:normalized_name"`
	CreatedBy      *uuid.UUID `gorm:"type:uuid;column:created_by"`
	CreatedAt      time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt      time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (AgentCatalogTag) TableName() string {
	return "agent_catalog_tags"
}

type AgentCatalogTagBinding struct {
	AgentID   uuid.UUID  `gorm:"type:uuid;primaryKey;column:agent_id"`
	TagID     uuid.UUID  `gorm:"type:uuid;primaryKey;column:tag_id"`
	CreatedBy *uuid.UUID `gorm:"type:uuid;column:created_by"`
	CreatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (AgentCatalogTagBinding) TableName() string {
	return "agent_catalog_tag_bindings"
}
