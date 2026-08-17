package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	GraphFlowRunItemOperationAdd     = "add"
	GraphFlowRunItemOperationCleanup = "cleanup"
)

type GraphFlowRunItem struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RunID             uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_graphflow_run_items_operation_document,priority:1" json:"run_id"`
	OrganizationID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"organization_id"`
	DatasetID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"dataset_id"`
	SourceRefID       *uuid.UUID `gorm:"type:uuid;index" json:"source_ref_id,omitempty"`
	SyncRunID         *uuid.UUID `gorm:"type:uuid;index" json:"sync_run_id,omitempty"`
	SyncBatchID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"sync_batch_id"`
	Operation         string     `gorm:"type:varchar(32);not null;uniqueIndex:idx_graphflow_run_items_operation_document,priority:2" json:"operation"`
	DocumentID        uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_graphflow_run_items_operation_document,priority:3" json:"document_id"`
	PairedDocumentID  *uuid.UUID `gorm:"type:uuid" json:"paired_document_id,omitempty"`
	AssetGenerationNo *int64     `json:"asset_generation_no,omitempty"`
	CreatedAt         time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (GraphFlowRunItem) TableName() string { return "graphflow_run_items" }

func (m *GraphFlowRunItem) BeforeCreate(_ *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
