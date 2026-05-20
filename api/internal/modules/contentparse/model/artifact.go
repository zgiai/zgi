package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Artifact struct {
	ID                    uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	SourceContentHash     string         `gorm:"type:varchar(255);not null;index:idx_content_parse_artifacts_hash_profile,priority:1;column:source_content_hash" json:"source_content_hash"`
	Profile               string         `gorm:"type:varchar(64);not null;index:idx_content_parse_artifacts_hash_profile,priority:2" json:"profile"`
	CanonicalIRVersion    string         `gorm:"type:varchar(64);not null;column:canonical_ir_version" json:"canonical_ir_version"`
	ProviderSignature     string         `gorm:"type:varchar(128);not null;column:provider_signature" json:"provider_signature"`
	ArtifactStorageKey    string         `gorm:"type:text;column:artifact_storage_key" json:"artifact_storage_key,omitempty"`
	DiagnosticsStorageKey string         `gorm:"type:text;column:diagnostics_storage_key" json:"diagnostics_storage_key,omitempty"`
	SummaryJSON           map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:summary_json" json:"summary_json,omitempty"`
	CreatedAt             time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt             time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (Artifact) TableName() string {
	return "content_parse_artifacts"
}

func (m *Artifact) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
