package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ReuseConsumerKnowledgeBase = "knowledge_base"
	ReuseConsumerDatabase      = "database"
	ReuseConsumerAgent         = "agent"
	ReuseConsumerWorkflow      = "workflow"

	ReuseArtifactDocumentVersion = "document_version"
	ReuseArtifactParseArtifact   = "parse_artifact"
	ReuseArtifactChunkArtifact   = "chunk_artifact"
	ReuseArtifactVectorArtifact  = "vector_artifact"
	ReuseArtifactGraphArtifact   = "graph_artifact"
	ReuseArtifactExtraction      = "extraction_artifact"
)

type ReuseEvent struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID  string         `gorm:"type:varchar(255);not null;index:idx_data_library_reuse_events_org_consumer,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID     *string        `gorm:"type:varchar(255);index:idx_data_library_reuse_events_workspace" json:"workspace_id,omitempty"`
	AssetID         uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_reuse_events_asset_created,priority:1;column:asset_id" json:"asset_id"`
	VersionID       *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_reuse_events_version;column:version_id" json:"version_id,omitempty"`
	ArtifactType    string         `gorm:"type:varchar(64);not null;column:artifact_type" json:"artifact_type"`
	ArtifactID      *uuid.UUID     `gorm:"type:uuid;index:idx_data_library_reuse_events_artifact;column:artifact_id" json:"artifact_id,omitempty"`
	ConsumerType    string         `gorm:"type:varchar(64);not null;index:idx_data_library_reuse_events_org_consumer,priority:2;column:consumer_type" json:"consumer_type"`
	ConsumerID      string         `gorm:"type:varchar(255);not null;index:idx_data_library_reuse_events_org_consumer,priority:3;column:consumer_id" json:"consumer_id"`
	ConsumerVersion string         `gorm:"type:varchar(255);column:consumer_version" json:"consumer_version,omitempty"`
	SavedSeconds    int64          `gorm:"not null;default:0;column:saved_seconds" json:"saved_seconds"`
	SavedCostMicros int64          `gorm:"not null;default:0;column:saved_cost_micros" json:"saved_cost_micros"`
	MetadataJSON    map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	CreatedBy       string         `gorm:"type:varchar(255);column:created_by" json:"created_by,omitempty"`
	CreatedAt       time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_data_library_reuse_events_asset_created,priority:2" json:"created_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (ReuseEvent) TableName() string {
	return "data_library_reuse_events"
}

func (m *ReuseEvent) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	return nil
}
