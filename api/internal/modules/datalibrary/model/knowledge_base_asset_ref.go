package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	KnowledgeBaseAssetRefStatusActive   = "active"
	KnowledgeBaseAssetRefStatusDisabled = "disabled"
)

type KnowledgeBaseAssetRef struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID     string         `gorm:"type:varchar(255);not null;index:idx_data_library_kb_asset_refs_org_dataset,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID        *string        `gorm:"type:varchar(255);index:idx_data_library_kb_asset_refs_workspace;column:workspace_id" json:"workspace_id,omitempty"`
	DatasetID          string         `gorm:"type:uuid;not null;index:idx_data_library_kb_asset_refs_org_dataset,priority:2;column:dataset_id" json:"dataset_id"`
	AssetID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_kb_asset_refs_asset;column:asset_id" json:"asset_id"`
	VersionID          uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_kb_asset_refs_version;column:version_id" json:"version_id"`
	ChunkArtifactSetID *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_kb_asset_refs_chunk_set;column:chunk_artifact_set_id" json:"chunk_artifact_set_id,omitempty"`
	VectorArtifactID   *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_kb_asset_refs_vector;column:vector_artifact_id" json:"vector_artifact_id,omitempty"`
	Status             string         `gorm:"type:varchar(32);not null;default:'active';index:idx_data_library_kb_asset_refs_status;column:status" json:"status"`
	MetadataJSON       map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	CreatedBy          string         `gorm:"type:varchar(255);column:created_by" json:"created_by,omitempty"`
	CreatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (KnowledgeBaseAssetRef) TableName() string {
	return "data_library_knowledge_base_asset_refs"
}

func (m *KnowledgeBaseAssetRef) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = KnowledgeBaseAssetRefStatusActive
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

func (m *KnowledgeBaseAssetRef) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
