package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	VectorArtifactStatusPending    = "pending"
	VectorArtifactStatusReady      = "ready"
	VectorArtifactStatusFailed     = "failed"
	VectorArtifactStatusRetired    = "retired"
	VectorArtifactStatusSuperseded = "superseded"
)

type VectorArtifact struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID     string         `gorm:"type:varchar(255);not null;index:idx_data_library_vector_artifacts_org_status,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID        *string        `gorm:"type:varchar(255);index:idx_data_library_vector_artifacts_workspace;column:workspace_id" json:"workspace_id,omitempty"`
	AssetID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_vector_artifacts_asset_created,priority:1;column:asset_id" json:"asset_id"`
	VersionID          uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_vector_artifacts_version;column:version_id" json:"version_id"`
	ChunkArtifactSetID uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_vector_artifacts_chunk_set;column:chunk_artifact_set_id" json:"chunk_artifact_set_id"`
	EmbeddingProvider  string         `gorm:"type:varchar(128);not null;column:embedding_provider" json:"embedding_provider"`
	EmbeddingModel     string         `gorm:"type:varchar(255);not null;column:embedding_model" json:"embedding_model"`
	EmbeddingDimension int            `gorm:"not null;default:0;column:embedding_dimension" json:"embedding_dimension"`
	VectorCollection   string         `gorm:"type:varchar(255);not null;column:vector_collection" json:"vector_collection"`
	VectorNamespace    string         `gorm:"type:varchar(255);column:vector_namespace" json:"vector_namespace,omitempty"`
	VectorCount        int64          `gorm:"not null;default:0;column:vector_count" json:"vector_count"`
	Status             string         `gorm:"type:varchar(32);not null;default:'pending';index:idx_data_library_vector_artifacts_org_status,priority:2" json:"status"`
	ContentHash        string         `gorm:"type:varchar(255);index:idx_data_library_vector_artifacts_content_hash;column:content_hash" json:"content_hash,omitempty"`
	MetadataJSON       map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	CreatedBy          string         `gorm:"type:varchar(255);column:created_by" json:"created_by,omitempty"`
	CreatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_data_library_vector_artifacts_asset_created,priority:2" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (VectorArtifact) TableName() string {
	return "data_library_vector_artifacts"
}

func (m *VectorArtifact) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = VectorArtifactStatusPending
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

func (m *VectorArtifact) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
