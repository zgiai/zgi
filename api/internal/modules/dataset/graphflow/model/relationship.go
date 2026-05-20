package model

import (
	"time"

	"github.com/google/uuid"
)

// Relationship represents a unique fact between two entities
type Relationship struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	KBID     uuid.UUID `gorm:"type:uuid;column:kb_id;not null" json:"kb_id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`

	HeadEntityID uuid.UUID `gorm:"type:uuid;not null" json:"head_entity_id"`
	TailEntityID uuid.UUID `gorm:"type:uuid;not null" json:"tail_entity_id"`

	RelationType string `gorm:"type:varchar(100);not null" json:"relation_type"`
	Weight       int    `gorm:"default:1" json:"weight"`

	GraphState   string     `gorm:"type:varchar(20);default:'pending'" json:"graph_state"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`

	IsDeleted bool       `gorm:"default:false" json:"is_deleted"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName specifies the table name for Relationship
func (Relationship) TableName() string {
	return "kb_relationships"
}
