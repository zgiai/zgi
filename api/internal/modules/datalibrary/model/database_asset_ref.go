package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	DatabaseAssetRefStatusActive   = "active"
	DatabaseAssetRefStatusDisabled = "disabled"
)

type DatabaseAssetRef struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID       string         `gorm:"type:varchar(255);not null;index:idx_data_library_db_asset_refs_org_source,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID          *string        `gorm:"type:varchar(255);index:idx_data_library_db_asset_refs_workspace;column:workspace_id" json:"workspace_id,omitempty"`
	DataSourceID         string         `gorm:"type:uuid;not null;index:idx_data_library_db_asset_refs_org_source,priority:2;column:data_source_id" json:"data_source_id"`
	TableID              *string        `gorm:"type:uuid;index:idx_data_library_db_asset_refs_table;column:table_id" json:"table_id,omitempty"`
	AssetID              uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_db_asset_refs_asset;column:asset_id" json:"asset_id"`
	VersionID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_db_asset_refs_version;column:version_id" json:"version_id"`
	ParseArtifactID      *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_db_asset_refs_parse;column:parse_artifact_id" json:"parse_artifact_id,omitempty"`
	ExtractionArtifactID *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_db_asset_refs_extraction;column:extraction_artifact_id" json:"extraction_artifact_id,omitempty"`
	Status               string         `gorm:"type:varchar(32);not null;default:'active';index:idx_data_library_db_asset_refs_status;column:status" json:"status"`
	MetadataJSON         map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	CreatedBy            string         `gorm:"type:varchar(255);column:created_by" json:"created_by,omitempty"`
	CreatedAt            time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (DatabaseAssetRef) TableName() string {
	return "data_library_database_asset_refs"
}

func (m *DatabaseAssetRef) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = DatabaseAssetRefStatusActive
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

func (m *DatabaseAssetRef) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
