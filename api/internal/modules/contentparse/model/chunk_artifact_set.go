package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ChunkArtifactSet struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	ParseArtifactID    *uuid.UUID     `gorm:"type:uuid;index:idx_content_parse_chunk_artifact_sets_parse_artifact,priority:1;column:parse_artifact_id" json:"parse_artifact_id,omitempty"`
	ParseRunID         *uuid.UUID     `gorm:"type:uuid;index:idx_content_parse_chunk_artifact_sets_parse_run,priority:1;column:parse_run_id" json:"parse_run_id,omitempty"`
	SourceContentHash  string         `gorm:"type:varchar(255);not null;index:idx_content_parse_chunk_artifact_sets_source_hash,priority:1;column:source_content_hash" json:"source_content_hash"`
	UseCase            string         `gorm:"type:varchar(32);not null;column:use_case" json:"use_case"`
	PlannerName        string         `gorm:"type:varchar(64);not null;column:planner_name" json:"planner_name"`
	ParentMode         string         `gorm:"type:varchar(64);column:parent_mode" json:"parent_mode,omitempty"`
	Segmentation       string         `gorm:"type:varchar(64)" json:"segmentation,omitempty"`
	ChunkerVersion     string         `gorm:"type:varchar(64);not null;column:chunker_version" json:"chunker_version"`
	Signature          string         `gorm:"type:varchar(255);not null;uniqueIndex:uq_content_parse_chunk_artifact_sets_signature" json:"signature"`
	Status             string         `gorm:"type:varchar(32);not null;default:'succeeded'" json:"status"`
	UnitCount          int            `gorm:"not null;default:0;column:unit_count" json:"unit_count"`
	ContentHash        string         `gorm:"type:varchar(255);not null;column:content_hash" json:"content_hash"`
	QualityJSON        map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:quality_json" json:"quality_json,omitempty"`
	SummaryJSON        map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:summary_json" json:"summary_json,omitempty"`
	ArtifactStorageKey string         `gorm:"type:text;column:artifact_storage_key" json:"artifact_storage_key,omitempty"`
	CreatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (ChunkArtifactSet) TableName() string {
	return "content_parse_chunk_artifact_sets"
}

func (m *ChunkArtifactSet) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
