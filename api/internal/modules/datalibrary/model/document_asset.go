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
)

type DocumentAsset struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID   string         `gorm:"type:varchar(255);not null;index:idx_data_library_assets_org_workspace_status,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID      *string        `gorm:"type:varchar(255);index:idx_data_library_assets_org_workspace_status,priority:2;column:workspace_id" json:"workspace_id,omitempty"`
	Title            string         `gorm:"type:text;not null" json:"title"`
	SourceFileID     string         `gorm:"type:varchar(255);not null;index:idx_data_library_assets_source_file;column:source_file_id" json:"source_file_id"`
	CurrentVersionID *uuid.UUID     `gorm:"type:uuid;column:current_version_id" json:"current_version_id,omitempty"`
	ContentHash      string         `gorm:"type:varchar(255);index:idx_data_library_assets_content_hash;column:content_hash" json:"content_hash,omitempty"`
	Status           string         `gorm:"type:varchar(32);not null;default:'archived';index:idx_data_library_assets_org_workspace_status,priority:3" json:"status"`
	ProcessingLevel  string         `gorm:"type:varchar(32);not null;default:'archive';column:processing_level" json:"processing_level"`
	QualityScore     *float64       `gorm:"column:quality_score" json:"quality_score,omitempty"`
	MetadataJSON     map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	PermissionPolicy map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:permission_policy" json:"permission_policy,omitempty"`
	CreatedBy        string         `gorm:"type:varchar(255);column:created_by" json:"created_by,omitempty"`
	CreatedAt        time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt        time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
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
