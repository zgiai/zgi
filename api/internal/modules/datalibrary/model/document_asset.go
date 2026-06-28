package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	DocumentAssetStatusArchived    = "archived"
	DocumentAssetStatusProcessing  = "processing"
	DocumentAssetStatusParsed      = "parsed"
	DocumentAssetStatusReady       = "ready"
	DocumentAssetStatusVectorized  = "vectorized"
	DocumentAssetStatusFull        = "full"
	DocumentAssetStatusNeedsReview = "needs_review"
	DocumentAssetStatusFailed      = "failed"

	DocumentProcessingLevelArchive   = "archive"
	DocumentProcessingLevelParse     = "parse"
	DocumentProcessingLevelSplit     = "split"
	DocumentProcessingLevelVectorize = "vectorize"
	DocumentProcessingLevelFull      = "full"

	DocumentAssetProductStatusStoredOnly  = "stored_only"
	DocumentAssetProductStatusParsing     = "parsing"
	DocumentAssetProductStatusConfirming  = "confirming"
	DocumentAssetProductStatusGenerating  = "generating"
	DocumentAssetProductStatusParseFailed = "parse_failed"
	DocumentAssetProductStatusReady       = "ready"

	DocumentAssetProcessingStageUpload    = "upload"
	DocumentAssetProcessingStageParse     = "parse"
	DocumentAssetProcessingStageReview    = "review"
	DocumentAssetProcessingStageChunk     = "chunk"
	DocumentAssetProcessingStageVectorize = "vectorize"
	DocumentAssetProcessingStageSync      = "sync"

	DocumentAssetVectorStatusNone     = "none"
	DocumentAssetVectorStatusIndexing = "indexing"
	DocumentAssetVectorStatusReady    = "ready"
	DocumentAssetVectorStatusFailed   = "failed"
)

type DocumentAsset struct {
	ID                        uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID            string         `gorm:"type:varchar(255);not null;index:idx_data_library_assets_org_workspace_status,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID               *string        `gorm:"type:varchar(255);index:idx_data_library_assets_org_workspace_status,priority:2;column:workspace_id" json:"workspace_id,omitempty"`
	Title                     string         `gorm:"type:text;not null" json:"title"`
	SourceFileID              string         `gorm:"type:varchar(255);not null;index:idx_data_library_assets_source_file;column:source_file_id" json:"source_file_id"`
	CurrentVersionID          *uuid.UUID     `gorm:"type:uuid;column:current_version_id" json:"current_version_id,omitempty"`
	ContentHash               string         `gorm:"type:varchar(255);index:idx_data_library_assets_content_hash;column:content_hash" json:"content_hash,omitempty"`
	Status                    string         `gorm:"type:varchar(32);not null;default:'archived';index:idx_data_library_assets_org_workspace_status,priority:3" json:"status"`
	ProcessingLevel           string         `gorm:"type:varchar(32);not null;default:'archive';column:processing_level" json:"processing_level"`
	ProductStatus             string         `gorm:"type:varchar(32);not null;default:'stored_only';index:idx_data_library_assets_product_status,priority:3;column:product_status" json:"product_status"`
	ProcessingStage           *string        `gorm:"type:varchar(32);column:processing_stage" json:"processing_stage,omitempty"`
	ProcessingProgress        int            `gorm:"not null;default:0;column:processing_progress" json:"processing_progress"`
	ActiveProcessingRequestID *uuid.UUID     `gorm:"type:uuid;column:active_processing_request_id" json:"active_processing_request_id,omitempty"`
	ProcessingRunID           *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_assets_processing_run;column:processing_run_id" json:"processing_run_id,omitempty"`
	GenerationNo              int64          `gorm:"not null;default:0;column:generation_no" json:"generation_no"`
	ParseArtifactID           *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_assets_parse_artifact;column:parse_artifact_id" json:"parse_artifact_id,omitempty"`
	ChunkArtifactSetID        *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_assets_chunk_artifact;column:chunk_artifact_set_id" json:"chunk_artifact_set_id,omitempty"`
	ChunkCount                int            `gorm:"not null;default:0;column:chunk_count" json:"chunk_count"`
	EmbeddingProvider         *string        `gorm:"type:varchar(128);column:embedding_provider" json:"embedding_provider,omitempty"`
	EmbeddingModel            *string        `gorm:"type:varchar(255);column:embedding_model" json:"embedding_model,omitempty"`
	EmbeddingDimension        *int           `gorm:"column:embedding_dimension" json:"embedding_dimension,omitempty"`
	VectorStatus              string         `gorm:"type:varchar(32);not null;default:'none';index:idx_data_library_assets_vector_status,priority:2;column:vector_status" json:"vector_status"`
	LastErrorCode             *string        `gorm:"type:varchar(128);column:last_error_code" json:"last_error_code,omitempty"`
	LastErrorMessage          *string        `gorm:"type:text;column:last_error_message" json:"last_error_message,omitempty"`
	QualityScore              *float64       `gorm:"column:quality_score" json:"quality_score,omitempty"`
	MetadataJSON              map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	PermissionPolicy          map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:permission_policy" json:"permission_policy,omitempty"`
	CreatedBy                 string         `gorm:"type:varchar(255);column:created_by" json:"created_by,omitempty"`
	CreatedAt                 time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt                 time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt                 gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (DocumentAsset) TableName() string {
	return "data_library_document_assets"
}

func (m *DocumentAsset) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = DocumentAssetStatusArchived
	}
	if m.ProcessingLevel == "" {
		m.ProcessingLevel = DocumentProcessingLevelArchive
	}
	if m.ProductStatus == "" {
		m.ProductStatus = DocumentAssetProductStatusStoredOnly
	}
	if m.VectorStatus == "" {
		m.VectorStatus = DocumentAssetVectorStatusNone
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}
	return nil
}

func (m *DocumentAsset) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
