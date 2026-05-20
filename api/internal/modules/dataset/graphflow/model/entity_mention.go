package model

import (
	"time"

	"github.com/google/uuid"
)

// EntityMention represents a raw entity mention extracted from a segment
type EntityMention struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	KBID      uuid.UUID `gorm:"type:uuid;column:kb_id;not null" json:"kb_id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	SegmentID uuid.UUID `gorm:"type:uuid;not null" json:"segment_id"`

	RawName    string  `gorm:"type:varchar(255);not null" json:"raw_name"`
	RawType    string  `gorm:"type:varchar(100)" json:"raw_type"`
	Confidence float64 `gorm:"default:1.0" json:"confidence"`

	EntityID *uuid.UUID `gorm:"type:uuid" json:"entity_id,omitempty"` // Link to resolved kb_entities
	Status   string     `gorm:"type:varchar(20);default:'pending'" json:"status"`

	IsDeleted bool       `gorm:"default:false" json:"is_deleted"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName specifies the table name
func (EntityMention) TableName() string {
	return "kb_entity_mentions"
}
