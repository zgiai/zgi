package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	GraphFlowRunStatusPending    = "pending"
	GraphFlowRunStatusProcessing = "processing"
	GraphFlowRunStatusReady      = "ready"
	GraphFlowRunStatusFailed     = "failed"
	GraphFlowRunStatusCancelled  = "cancelled"
	GraphFlowRunStatusSuperseded = "superseded"
)

const (
	GraphFlowRunModeBuild    = "build"
	GraphFlowRunModeBackfill = "backfill"
	GraphFlowRunModeRebuild  = "rebuild"
	GraphFlowRunModeCleanup  = "cleanup"
)

type GraphFlowRun struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"organization_id"`
	WorkspaceID          *uuid.UUID `gorm:"type:uuid;index" json:"workspace_id,omitempty"`
	DatasetID            uuid.UUID  `gorm:"type:uuid;not null;index:idx_graphflow_runs_dataset_status,priority:1;uniqueIndex:idx_graphflow_runs_dataset_idempotency,priority:1" json:"dataset_id"`
	DocumentID           *uuid.UUID `gorm:"type:uuid;index:idx_graphflow_runs_source_document,priority:2" json:"document_id,omitempty"`
	SourceRefID          *uuid.UUID `gorm:"type:uuid;index:idx_graphflow_runs_source_document,priority:1" json:"source_ref_id,omitempty"`
	SyncRunID            *uuid.UUID `gorm:"type:uuid;index" json:"sync_run_id,omitempty"`
	AssetGenerationNo    *int64     `json:"asset_generation_no,omitempty"`
	GraphRevision        int64      `gorm:"not null" json:"graph_revision"`
	EmbeddingProvider    string     `gorm:"column:embedding_model_provider;type:varchar(255);not null;default:''" json:"embedding_model_provider"`
	EmbeddingModel       string     `gorm:"type:varchar(255);not null;default:''" json:"embedding_model"`
	EmbeddingDimension   int        `gorm:"not null;default:0" json:"embedding_dimension"`
	EmbeddingFingerprint string     `gorm:"type:varchar(512);not null;default:''" json:"embedding_fingerprint"`
	Trigger              string     `gorm:"type:varchar(32);not null" json:"trigger"`
	Mode                 string     `gorm:"type:varchar(32);not null" json:"mode"`
	Status               string     `gorm:"type:varchar(32);not null;default:'pending';index:idx_graphflow_runs_dataset_status,priority:2" json:"status"`
	Progress             int        `gorm:"not null;default:0" json:"progress"`
	IdempotencyKey       string     `gorm:"type:varchar(255);not null;uniqueIndex:idx_graphflow_runs_dataset_idempotency,priority:2" json:"idempotency_key"`
	ErrorCode            *string    `gorm:"type:varchar(128)" json:"error_code,omitempty"`
	ErrorMessage         *string    `gorm:"type:text" json:"error_message,omitempty"`
	AttemptCount         int        `gorm:"not null;default:0" json:"attempt_count"`
	LeaseExpiresAt       *time.Time `json:"lease_expires_at,omitempty"`
	HeartbeatAt          *time.Time `json:"heartbeat_at,omitempty"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	FinishedAt           *time.Time `json:"finished_at,omitempty"`
	CreatedAt            time.Time  `gorm:"autoCreateTime;index:idx_graphflow_runs_dataset_status,priority:3" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (GraphFlowRun) TableName() string {
	return "graphflow_runs"
}

func (m *GraphFlowRun) BeforeCreate(_ *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = GraphFlowRunStatusPending
	}
	return nil
}
