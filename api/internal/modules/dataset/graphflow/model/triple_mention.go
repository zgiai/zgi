package model

import (
	"time"

	"github.com/google/uuid"
)

// TripleMention represents a raw relationship triple extracted from a segment
type TripleMention struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	KBID      uuid.UUID `gorm:"type:uuid;column:kb_id;not null" json:"kb_id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	SegmentID uuid.UUID `gorm:"type:uuid;not null" json:"segment_id"`

	RawSubject   string `gorm:"type:varchar(255);not null" json:"raw_subject"`
	RawPredicate string `gorm:"type:varchar(255);not null" json:"raw_predicate"`
	RawObject    string `gorm:"type:varchar(255);not null" json:"raw_object"`

	HeadEntityID *uuid.UUID `gorm:"type:uuid" json:"head_entity_id,omitempty"`
	TailEntityID *uuid.UUID `gorm:"type:uuid" json:"tail_entity_id,omitempty"`

	Status string `gorm:"type:varchar(20);default:'pending'" json:"status"`

	IsDeleted bool       `gorm:"default:false" json:"is_deleted"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName specifies the table name
func (TripleMention) TableName() string {
	return "kb_triple_mentions"
}
