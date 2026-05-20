package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ProcessingRequestStatusPlanned   = "planned"
	ProcessingRequestStatusQueued    = "queued"
	ProcessingRequestStatusRunning   = "running"
	ProcessingRequestStatusCompleted = "completed"
	ProcessingRequestStatusFailed    = "failed"
	ProcessingRequestStatusCancelled = "cancelled"
)

type ProcessingRequest struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID    string         `gorm:"type:varchar(255);not null;index:idx_data_library_processing_requests_org_status,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID       *string        `gorm:"type:varchar(255);index:idx_data_library_processing_requests_workspace;column:workspace_id" json:"workspace_id,omitempty"`
	AssetID           uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_processing_requests_asset_created,priority:1;column:asset_id" json:"asset_id"`
	TargetLevel       string         `gorm:"type:varchar(32);not null;column:target_level" json:"target_level"`
	Status            string         `gorm:"type:varchar(32);not null;default:'planned';index:idx_data_library_processing_requests_org_status,priority:2" json:"status"`
	RequestedBy       string         `gorm:"type:varchar(255);column:requested_by" json:"requested_by,omitempty"`
	Force             bool           `gorm:"not null;default:false" json:"force"`
	PlanJSON          map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:plan_json" json:"plan_json,omitempty"`
	RequestMetadata   map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:request_metadata" json:"request_metadata,omitempty"`
	ExecutionMetadata map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:execution_metadata" json:"execution_metadata,omitempty"`
	ExecutorKey       string         `gorm:"type:varchar(255);column:executor_key" json:"executor_key,omitempty"`
	ErrorCode         string         `gorm:"type:varchar(128);column:error_code" json:"error_code,omitempty"`
	ErrorMessage      string         `gorm:"type:text;column:error_message" json:"error_message,omitempty"`
	AttemptCount      int            `gorm:"not null;default:0;column:attempt_count" json:"attempt_count"`
	QueuedAt          *time.Time     `gorm:"column:queued_at" json:"queued_at,omitempty"`
	StartedAt         *time.Time     `gorm:"column:started_at" json:"started_at,omitempty"`
	CompletedAt       *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
	FailedAt          *time.Time     `gorm:"column:failed_at" json:"failed_at,omitempty"`
	CanceledAt        *time.Time     `gorm:"column:cancelled_at" json:"cancelled_at,omitempty"`
	CreatedAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_data_library_processing_requests_asset_created,priority:2" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (ProcessingRequest) TableName() string {
	return "data_library_processing_requests"
}

func (m *ProcessingRequest) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = ProcessingRequestStatusPlanned
	}
	if m.PlanJSON == nil {
		m.PlanJSON = map[string]any{}
	}
	if m.RequestMetadata == nil {
		m.RequestMetadata = map[string]any{}
	}
	if m.ExecutionMetadata == nil {
		m.ExecutionMetadata = map[string]any{}
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}
	return nil
}

func (m *ProcessingRequest) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
