package model

import (
	"time"

	"github.com/google/uuid"
)

// TripleMention represents a raw relationship triple extracted from a segment
type TripleMention struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	KBID                uuid.UUID  `gorm:"type:uuid;column:kb_id;not null;index:idx_triple_mentions_kb_relationship_active,priority:1,where:is_deleted = false AND relationship_id IS NOT NULL" json:"kb_id"`
	TenantID            uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	SegmentID           uuid.UUID  `gorm:"type:uuid;not null" json:"segment_id"`
	OrganizationID      uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	SourceRefID         *uuid.UUID `gorm:"type:uuid;index" json:"source_ref_id,omitempty"`
	DocumentID          *uuid.UUID `gorm:"type:uuid;index" json:"document_id,omitempty"`
	RunID               *uuid.UUID `gorm:"type:uuid;index" json:"run_id,omitempty"`
	RelationshipID      *uuid.UUID `gorm:"type:uuid;index:idx_triple_mentions_kb_relationship_active,priority:2,where:is_deleted = false AND relationship_id IS NOT NULL" json:"relationship_id,omitempty"`
	EvidenceFingerprint string     `gorm:"type:varchar(128);not null;default:''" json:"evidence_fingerprint"`

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
