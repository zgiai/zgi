package model

import (
	"time"

	"github.com/google/uuid"
)

// Entity represents a canonical entity in the Knowledge Graph
type Entity struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	KBID     uuid.UUID `gorm:"type:uuid;column:kb_id;not null" json:"kb_id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`

	Name          string `gorm:"type:varchar(255);not null" json:"name"`
	CanonicalName string `gorm:"type:varchar(255);not null" json:"canonical_name"`
	Type          string `gorm:"type:varchar(100);not null" json:"type"`
	Description   string `gorm:"type:text" json:"description"`

	SourceCount int      `gorm:"default:1" json:"source_count"`
	MergedIDs   []string `gorm:"type:jsonb;serializer:json;default:'[]'" json:"merged_ids"`

	EmbeddingID string `gorm:"type:varchar(255)" json:"embedding_id"`
	GraphNodeID string `gorm:"type:varchar(255)" json:"graph_node_id"`

	VectorState string `gorm:"type:varchar(20);default:'pending'" json:"vector_state"`
	GraphState  string `gorm:"type:varchar(20);default:'pending'" json:"graph_state"`
	SyncErrorLog string `gorm:"type:text" json:"sync_error_log"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	
	IsDeleted bool       `gorm:"default:false" json:"is_deleted"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// TableName specifies the table name for Entity
func (Entity) TableName() string {
	return "kb_entities"
}
