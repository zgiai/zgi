package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ChunkingRun struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	ParseRunID         uuid.UUID      `gorm:"type:uuid;not null;index:idx_content_parse_chunking_parse_created,priority:1;column:parse_run_id" json:"parse_run_id"`
	ChunkArtifactSetID *uuid.UUID     `gorm:"type:uuid;index:idx_content_parse_chunking_artifact_set;column:chunk_artifact_set_id" json:"chunk_artifact_set_id,omitempty"`
	UseCase            string         `gorm:"type:varchar(32);not null;index:idx_content_parse_chunking_use_case_created,priority:1;column:use_case" json:"use_case"`
	PlannerName        string         `gorm:"type:varchar(64);not null;column:planner_name" json:"planner_name"`
	ParentMode         string         `gorm:"type:varchar(64);column:parent_mode" json:"parent_mode,omitempty"`
	Segmentation       string         `gorm:"type:varchar(64)" json:"segmentation,omitempty"`
	UnitCount          int            `gorm:"not null;default:0;column:unit_count" json:"unit_count"`
	PlanJSON           map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:plan_json" json:"plan_json,omitempty"`
	ArtifactStorageKey string         `gorm:"type:text;column:artifact_storage_key" json:"artifact_storage_key,omitempty"`
	CreatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_content_parse_chunking_parse_created,priority:2;index:idx_content_parse_chunking_use_case_created,priority:2" json:"created_at"`
}

func (ChunkingRun) TableName() string {
	return "content_parse_chunking_runs"
}

func (m *ChunkingRun) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	return nil
}
