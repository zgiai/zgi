package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ExtractionArtifactStatusPending    = "pending"
	ExtractionArtifactStatusReady      = "ready"
	ExtractionArtifactStatusFailed     = "failed"
	ExtractionArtifactStatusRetired    = "retired"
	ExtractionArtifactStatusSuperseded = "superseded"
)

type ExtractionArtifact struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID    string         `gorm:"type:varchar(255);not null;index:idx_data_library_extraction_artifacts_org_status,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID       *string        `gorm:"type:varchar(255);index:idx_data_library_extraction_artifacts_workspace;column:workspace_id" json:"workspace_id,omitempty"`
	AssetID           uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_extraction_artifacts_asset_created,priority:1;column:asset_id" json:"asset_id"`
	VersionID         uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_extraction_artifacts_version;column:version_id" json:"version_id"`
	ParseArtifactID   *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_extraction_artifacts_parse;column:parse_artifact_id" json:"parse_artifact_id,omitempty"`
	DataSourceID      *string        `gorm:"type:uuid;index:idx_data_library_extraction_artifacts_source;column:data_source_id" json:"data_source_id,omitempty"`
	TableID           *string        `gorm:"type:uuid;index:idx_data_library_extraction_artifacts_table;column:table_id" json:"table_id,omitempty"`
	SchemaName        string         `gorm:"type:varchar(255);column:schema_name" json:"schema_name,omitempty"`
	SchemaHash        string         `gorm:"type:varchar(255);index:idx_data_library_extraction_artifacts_schema_hash;column:schema_hash" json:"schema_hash,omitempty"`
	ExtractorProvider string         `gorm:"type:varchar(128);column:extractor_provider" json:"extractor_provider,omitempty"`
	ExtractorModel    string         `gorm:"type:varchar(255);column:extractor_model" json:"extractor_model,omitempty"`
	RecordCount       int64          `gorm:"not null;default:0;column:record_count" json:"record_count"`
	FieldCount        int64          `gorm:"not null;default:0;column:field_count" json:"field_count"`
	EvidenceCount     int64          `gorm:"not null;default:0;column:evidence_count" json:"evidence_count"`
	Status            string         `gorm:"type:varchar(32);not null;default:'pending';index:idx_data_library_extraction_artifacts_org_status,priority:2" json:"status"`
	QualityScore      *float64       `gorm:"column:quality_score" json:"quality_score,omitempty"`
	ContentHash       string         `gorm:"type:varchar(255);index:idx_data_library_extraction_artifacts_content_hash;column:content_hash" json:"content_hash,omitempty"`
	OutputURI         string         `gorm:"type:text;column:output_uri" json:"output_uri,omitempty"`
	MetadataJSON      map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	CreatedBy         string         `gorm:"type:varchar(255);column:created_by" json:"created_by,omitempty"`
	CreatedAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_data_library_extraction_artifacts_asset_created,priority:2" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (ExtractionArtifact) TableName() string {
	return "data_library_extraction_artifacts"
}

func (m *ExtractionArtifact) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = ExtractionArtifactStatusPending
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

func (m *ExtractionArtifact) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
