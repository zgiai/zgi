package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ParseRun struct {
	ID                     uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	WorkspaceID            *uuid.UUID     `gorm:"type:uuid;column:workspace_id;index:idx_content_parse_runs_workspace_created,priority:1" json:"workspace_id,omitempty"`
	DatasetID              *uuid.UUID     `gorm:"type:uuid;column:dataset_id;index:idx_content_parse_runs_dataset_document_created,priority:1" json:"dataset_id,omitempty"`
	DocumentID             *uuid.UUID     `gorm:"type:uuid;column:document_id;index:idx_content_parse_runs_dataset_document_created,priority:2" json:"document_id,omitempty"`
	FileID                 *uuid.UUID     `gorm:"type:uuid;column:file_id" json:"file_id,omitempty"`
	ArtifactID             *uuid.UUID     `gorm:"type:uuid;column:artifact_id" json:"artifact_id,omitempty"`
	SourceType             string         `gorm:"type:varchar(32);not null;column:source_type" json:"source_type"`
	SourceRef              string         `gorm:"type:text;column:source_ref" json:"source_ref,omitempty"`
	FileName               string         `gorm:"type:text;column:file_name" json:"file_name,omitempty"`
	Intent                 string         `gorm:"type:varchar(32);not null" json:"intent"`
	Profile                string         `gorm:"type:varchar(64);not null" json:"profile"`
	PolicyKey              string         `gorm:"type:varchar(64);column:policy_key" json:"policy_key,omitempty"`
	RoutePolicyID          *uuid.UUID     `gorm:"type:uuid;column:route_policy_id" json:"route_policy_id,omitempty"`
	RequestedProviderKey   string         `gorm:"type:varchar(64);column:requested_provider_key" json:"requested_provider_key,omitempty"`
	PlannedProviderOrder   []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"planned_provider_order,omitempty"`
	AttemptedProviderOrder []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"attempted_provider_order,omitempty"`
	FinalProviderKey       string         `gorm:"type:varchar(64);index:idx_content_parse_runs_provider_created,priority:1;column:final_provider_key" json:"final_provider_key,omitempty"`
	AdapterName            string         `gorm:"type:varchar(64);column:adapter_name" json:"adapter_name,omitempty"`
	EngineName             string         `gorm:"type:varchar(64);column:engine_name" json:"engine_name,omitempty"`
	Status                 string         `gorm:"type:varchar(32);not null;index:idx_content_parse_runs_status_quality_created,priority:1" json:"status"`
	QualityLevel           string         `gorm:"type:varchar(32);not null;index:idx_content_parse_runs_status_quality_created,priority:2;column:quality_level" json:"quality_level"`
	FallbackUsed           bool           `gorm:"not null;default:false;column:fallback_used" json:"fallback_used"`
	DurationMS             *int           `gorm:"column:duration_ms" json:"duration_ms,omitempty"`
	ArtifactStorageKey     string         `gorm:"type:text;column:artifact_storage_key" json:"artifact_storage_key,omitempty"`
	DiagnosticsStorageKey  string         `gorm:"type:text;column:diagnostics_storage_key" json:"diagnostics_storage_key,omitempty"`
	SummaryJSON            map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:summary_json" json:"summary_json,omitempty"`
	CreatedAt              time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_content_parse_runs_workspace_created,priority:2;index:idx_content_parse_runs_dataset_document_created,priority:3;index:idx_content_parse_runs_status_quality_created,priority:3;index:idx_content_parse_runs_provider_created,priority:2" json:"created_at"`
}

func (ParseRun) TableName() string {
	return "content_parse_runs"
}

func (m *ParseRun) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	return nil
}
