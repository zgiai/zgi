package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ChildChunk represents a child chunk in hierarchical document processing
type ChildChunk struct {
	ID             string     `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	OrganizationID string     `json:"organization_id" gorm:"type:uuid;not null"`
	DatasetID      string     `json:"dataset_id" gorm:"type:uuid;not null"`
	DocumentID     string     `json:"document_id" gorm:"type:uuid;not null"`
	SegmentID      string     `json:"segment_id" gorm:"type:uuid;not null"`
	Position       int        `json:"position" gorm:"not null"`
	Content        string     `json:"content" gorm:"type:text;not null"`
	WordCount      int        `json:"word_count" gorm:"not null"`
	IndexNodeID    *string    `json:"index_node_id" gorm:"type:varchar(255)"`
	IndexNodeHash  *string    `json:"index_node_hash" gorm:"type:varchar(255)"`
	Type           string     `json:"type" gorm:"type:varchar(255);not null;default:'automatic'"`
	CreatedBy      string     `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt      time.Time  `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedBy      *string    `json:"updated_by" gorm:"type:uuid"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	IndexingAt     *time.Time `json:"indexing_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	Error          *string    `json:"error" gorm:"type:text"`
}

// TableName specifies the table name for ChildChunk
func (ChildChunk) TableName() string {
	return "child_chunks"
}

// BeforeCreate is a GORM hook that sets the ID before creating
func (cc *ChildChunk) BeforeCreate(tx *gorm.DB) error {
	if cc.ID == "" {
		cc.ID = uuid.New().String()
	}
	if cc.CreatedAt.IsZero() {
		cc.CreatedAt = time.Now()
	}
	if cc.UpdatedAt.IsZero() {
		cc.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate is a GORM hook that updates the UpdatedAt field
func (cc *ChildChunk) BeforeUpdate(tx *gorm.DB) error {
	cc.UpdatedAt = time.Now()
	return nil
}

// Constants for child chunk types
const (
	ChildChunkTypeAutomatic = "automatic"
	ChildChunkTypeManual    = "manual"
)

// Constants for child chunk status
const (
	ChildChunkStatusWaiting   = "waiting"
	ChildChunkStatusIndexing  = "indexing"
	ChildChunkStatusCompleted = "completed"
	ChildChunkStatusError     = "error"
)
