package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PlaygroundRun struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID          *uuid.UUID     `gorm:"type:uuid;column:workspace_id;index:idx_content_parse_playground_runs_workspace_created,priority:1" json:"workspace_id,omitempty"`
	AccountID            *uuid.UUID     `gorm:"type:uuid;column:account_id" json:"account_id,omitempty"`
	FileName             string         `gorm:"type:text;not null;column:file_name" json:"file_name"`
	FileSize             int64          `gorm:"not null;default:0;column:file_size" json:"file_size"`
	SourceContentHash    string         `gorm:"type:varchar(255);not null;column:source_content_hash;index:idx_content_parse_playground_runs_hash_created,priority:1;index:idx_content_parse_playground_runs_hash_provider_created,priority:1" json:"source_content_hash"`
	SourceStorageKey     string         `gorm:"type:text;column:source_storage_key" json:"-"`
	SourceStorageType    string         `gorm:"type:varchar(64);column:source_storage_type" json:"source_storage_type,omitempty"`
	SourceMimeType       string         `gorm:"type:varchar(128);column:source_mime_type" json:"source_mime_type,omitempty"`
	SourceFileExt        string         `gorm:"type:varchar(32);column:source_file_ext" json:"source_file_ext,omitempty"`
	RequestedProviderKey string         `gorm:"type:varchar(64);not null;column:requested_provider_key" json:"requested_provider_key"`
	FinalProviderKey     string         `gorm:"type:varchar(64);column:final_provider_key;index:idx_content_parse_playground_runs_hash_provider_created,priority:2" json:"final_provider_key,omitempty"`
	AdapterName          string         `gorm:"type:varchar(64);column:adapter_name" json:"adapter_name,omitempty"`
	EngineName           string         `gorm:"type:varchar(64);column:engine_name" json:"engine_name,omitempty"`
	Profile              string         `gorm:"type:varchar(64);not null;column:profile;index:idx_content_parse_playground_runs_hash_provider_created,priority:3" json:"profile"`
	OCREngine            string         `gorm:"type:varchar(64);column:ocr_engine" json:"ocr_engine,omitempty"`
	Status               string         `gorm:"type:varchar(32);not null;column:status" json:"status"`
	QualityLevel         string         `gorm:"type:varchar(32);not null;column:quality_level" json:"quality_level"`
	FallbackUsed         bool           `gorm:"not null;default:false;column:fallback_used" json:"fallback_used"`
	DurationMS           *int           `gorm:"column:duration_ms" json:"duration_ms,omitempty"`
	ArtifactJSON         map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:artifact_json" json:"artifact_json,omitempty"`
	RoutePlanJSON        map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:route_plan_json" json:"route_plan_json,omitempty"`
	ChunkSourceJSON      map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:chunk_source_json" json:"chunk_source_json,omitempty"`
	ChunkPlanJSON        map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:chunk_plan_json" json:"chunk_plan_json,omitempty"`
	QualitySummaryJSON   map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:quality_summary_json" json:"quality_summary_json,omitempty"`
	SummaryJSON          map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:summary_json" json:"summary_json,omitempty"`
	ShareToken           string         `gorm:"type:varchar(64);not null;uniqueIndex:uq_content_parse_playground_runs_share_token;column:share_token" json:"share_token"`
	IsShareEnabled       bool           `gorm:"not null;column:is_share_enabled" json:"is_share_enabled"`
	CreatedAt            time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_content_parse_playground_runs_workspace_created,priority:2;index:idx_content_parse_playground_runs_hash_created,priority:2;index:idx_content_parse_playground_runs_hash_provider_created,priority:4" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt            gorm.DeletedAt `gorm:"index:idx_content_parse_playground_runs_deleted_at" json:"deleted_at,omitempty"`
}

func (PlaygroundRun) TableName() string {
	return "content_parse_playground_runs"
}

func (m *PlaygroundRun) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = m.CreatedAt
	}
	return nil
}
