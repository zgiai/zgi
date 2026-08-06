package model

import (
	"time"

	"github.com/google/uuid"
)

// GraphFlowTask tracks async tasks for GraphFlow
type GraphFlowTask struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	KBID     uuid.UUID `gorm:"type:uuid;column:kb_id;not null" json:"kb_id"` // Maps to datasets(id)
	// DocumentID is a durable source identifier. Cleanup tasks may outlive the documents row.
	DocumentID  uuid.UUID  `gorm:"type:uuid;not null" json:"document_id"`
	SegmentID   *uuid.UUID `gorm:"type:uuid" json:"segment_id,omitempty"`
	RunID       *uuid.UUID `gorm:"type:uuid;index" json:"run_id,omitempty"`
	RunItemID   *uuid.UUID `gorm:"type:uuid;index" json:"run_item_id,omitempty"`
	SourceRefID *uuid.UUID `gorm:"type:uuid;index" json:"source_ref_id,omitempty"`

	TaskType           string `gorm:"type:varchar(50);not null" json:"task_type"`                // 'extraction', 'alignment', 'graph_sync', etc.
	ExtractionStrategy string `gorm:"type:varchar(20);default:'llm'" json:"extraction_strategy"` // GraphFlow uses 'llm'.
	Status             string `gorm:"type:varchar(50);not null;default:'pending'" json:"status"`
	Progress           int    `gorm:"default:0" json:"progress"`

	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	ErrorMessage   string     `gorm:"type:text" json:"error_message,omitempty"`
	RetryCount     int        `gorm:"default:0" json:"retry_count"`
	AttemptNo      int        `gorm:"not null;default:0" json:"attempt_no"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	HeartbeatAt    *time.Time `json:"heartbeat_at,omitempty"`
	ErrorCode      string     `gorm:"type:varchar(128)" json:"error_code,omitempty"`

	Metadata map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName specifies the table name
func (GraphFlowTask) TableName() string {
	return "graphflow_tasks"
}
