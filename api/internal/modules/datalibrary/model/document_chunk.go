package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	DocumentChunkTypeParent = "parent"
	DocumentChunkTypeChild  = "child"
	DocumentChunkTypeManual = "manual"
	DocumentChunkTypeAuto   = "auto"

	DocumentChunkStatusReady      = "ready"
	DocumentChunkStatusReindexing = "reindexing"
	DocumentChunkStatusError      = "error"
	DocumentChunkStatusDeleted    = "deleted"
)

type DocumentChunk struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID     string         `gorm:"type:varchar(255);not null;index:idx_data_library_document_chunks_org_asset_generation,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID        *string        `gorm:"type:varchar(255);column:workspace_id" json:"workspace_id,omitempty"`
	AssetID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_document_chunks_org_asset_generation,priority:2;index:idx_data_library_document_chunks_parent_position,priority:1;index:idx_data_library_document_chunks_asset_enabled_status,priority:1;column:asset_id" json:"asset_id"`
	ProcessingRunID    uuid.UUID      `gorm:"type:uuid;not null;column:processing_run_id" json:"processing_run_id"`
	GenerationNo       int64          `gorm:"not null;index:idx_data_library_document_chunks_org_asset_generation,priority:3;column:generation_no" json:"generation_no"`
	ChunkArtifactSetID *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_document_chunks_artifact_set;column:chunk_artifact_set_id" json:"chunk_artifact_set_id,omitempty"`
	ParentChunkID      *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_document_chunks_parent_position,priority:2;column:parent_chunk_id" json:"parent_chunk_id,omitempty"`
	Position           int            `gorm:"not null;default:0;index:idx_data_library_document_chunks_org_asset_generation,priority:4;index:idx_data_library_document_chunks_parent_position,priority:3;column:position" json:"position"`
	ChunkType          string         `gorm:"type:varchar(32);not null;column:chunk_type" json:"chunk_type"`
	Content            string         `gorm:"type:text;not null;column:content" json:"content"`
	ContentHash        string         `gorm:"type:varchar(255);not null;column:content_hash" json:"content_hash"`
	SourceLocatorJSON  map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:source_locator_json" json:"source_locator_json,omitempty"`
	Enabled            bool           `gorm:"not null;default:true;index:idx_data_library_document_chunks_asset_enabled_status,priority:2;column:enabled" json:"enabled"`
	Status             string         `gorm:"type:varchar(32);not null;default:'ready';index:idx_data_library_document_chunks_asset_enabled_status,priority:3;column:status" json:"status"`
	MetadataJSON       map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	CreatedBy          string         `gorm:"type:varchar(255);column:created_by" json:"created_by,omitempty"`
	UpdatedBy          string         `gorm:"type:varchar(255);column:updated_by" json:"updated_by,omitempty"`
	CreatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (DocumentChunk) TableName() string {
	return "data_library_document_chunks"
}

func (m *DocumentChunk) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.ChunkType == "" {
		m.ChunkType = DocumentChunkTypeAuto
	}
	if m.Status == "" {
		m.Status = DocumentChunkStatusReady
	}
	if m.SourceLocatorJSON == nil {
		m.SourceLocatorJSON = map[string]any{}
	}
	if m.MetadataJSON == nil {
		m.MetadataJSON = map[string]any{}
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}
	return nil
}

func (m *DocumentChunk) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
