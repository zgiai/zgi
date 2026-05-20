package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	DocumentVersionStatusArchived    = "archived"
	DocumentVersionStatusProcessing  = "processing"
	DocumentVersionStatusParsed      = "parsed"
	DocumentVersionStatusReady       = "ready"
	DocumentVersionStatusVectorized  = "vectorized"
	DocumentVersionStatusNeedsReview = "needs_review"
	DocumentVersionStatusFailed      = "failed"
)

type DocumentVersion struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AssetID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_versions_asset_created,priority:1;column:asset_id" json:"asset_id"`
	VersionNo          int            `gorm:"not null;column:version_no" json:"version_no"`
	SourceFileID       string         `gorm:"type:varchar(255);not null;index:idx_data_library_versions_source_file;column:source_file_id" json:"source_file_id"`
	ContentHash        string         `gorm:"type:varchar(255);index:idx_data_library_versions_content_hash;column:content_hash" json:"content_hash,omitempty"`
	FileName           string         `gorm:"type:text;column:file_name" json:"file_name,omitempty"`
	FileSize           int64          `gorm:"column:file_size" json:"file_size,omitempty"`
	MimeType           string         `gorm:"type:varchar(255);column:mime_type" json:"mime_type,omitempty"`
	ParseArtifactID    *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_versions_parse_artifact;column:parse_artifact_id" json:"parse_artifact_id,omitempty"`
	ChunkArtifactSetID *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_versions_chunk_artifact;column:chunk_artifact_set_id" json:"chunk_artifact_set_id,omitempty"`
	Status             string         `gorm:"type:varchar(32);not null;default:'archived'" json:"status"`
	QualityScore       *float64       `gorm:"column:quality_score" json:"quality_score,omitempty"`
	UploadedBy         string         `gorm:"type:varchar(255);column:uploaded_by" json:"uploaded_by,omitempty"`
	MetadataJSON       map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	CreatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_data_library_versions_asset_created,priority:2" json:"created_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (DocumentVersion) TableName() string {
	return "data_library_document_versions"
}

func (m *DocumentVersion) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = DocumentVersionStatusArchived
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	return nil
}
