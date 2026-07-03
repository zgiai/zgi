package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ParseConfirmationItemTypeLowConfidenceText = "low_confidence_text"
	ParseConfirmationItemTypeTable             = "table"
	ParseConfirmationItemTypeImageOCR          = "image_ocr"
	ParseConfirmationItemTypeStructure         = "structure"
	ParseConfirmationItemTypeOther             = "other"

	ParseConfirmationItemStatusPending = "pending"
	ParseConfirmationItemStatusKept    = "kept"
	ParseConfirmationItemStatusEdited  = "edited"
	ParseConfirmationItemStatusIgnored = "ignored"
)

type ParseConfirmationItem struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID    string         `gorm:"type:varchar(255);not null;index:idx_data_library_parse_confirm_org_asset_status,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID       *string        `gorm:"type:varchar(255);column:workspace_id" json:"workspace_id,omitempty"`
	AssetID           uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_parse_confirm_org_asset_status,priority:2;index:idx_data_library_parse_confirm_asset_generation,priority:1;column:asset_id" json:"asset_id"`
	ProcessingRunID   uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_parse_confirm_run_status,priority:1;column:processing_run_id" json:"processing_run_id"`
	GenerationNo      int64          `gorm:"not null;index:idx_data_library_parse_confirm_asset_generation,priority:2;column:generation_no" json:"generation_no"`
	ItemType          string         `gorm:"type:varchar(64);not null;column:item_type" json:"item_type"`
	Status            string         `gorm:"type:varchar(32);not null;default:'pending';index:idx_data_library_parse_confirm_org_asset_status,priority:3;index:idx_data_library_parse_confirm_run_status,priority:2;index:idx_data_library_parse_confirm_asset_generation,priority:3" json:"status"`
	SourceLocatorJSON map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:source_locator_json" json:"source_locator_json,omitempty"`
	OriginalContent   string         `gorm:"type:text;not null;column:original_content" json:"original_content"`
	SuggestedContent  *string        `gorm:"type:text;column:suggested_content" json:"suggested_content,omitempty"`
	FinalContent      *string        `gorm:"type:text;column:final_content" json:"final_content,omitempty"`
	Confidence        *float64       `gorm:"column:confidence" json:"confidence,omitempty"`
	ReviewReason      *string        `gorm:"type:varchar(128);column:review_reason" json:"review_reason,omitempty"`
	CreatedBy         string         `gorm:"type:varchar(255);column:created_by" json:"created_by,omitempty"`
	UpdatedBy         string         `gorm:"type:varchar(255);column:updated_by" json:"updated_by,omitempty"`
	ResolvedAt        *time.Time     `gorm:"column:resolved_at" json:"resolved_at,omitempty"`
	CreatedAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_data_library_parse_confirm_org_asset_status,priority:4;index:idx_data_library_parse_confirm_run_status,priority:3;index:idx_data_library_parse_confirm_asset_generation,priority:4" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (ParseConfirmationItem) TableName() string {
	return "data_library_parse_confirmation_items"
}

func (m *ParseConfirmationItem) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.ItemType == "" {
		m.ItemType = ParseConfirmationItemTypeOther
	}
	if m.Status == "" {
		m.Status = ParseConfirmationItemStatusPending
	}
	if m.SourceLocatorJSON == nil {
		m.SourceLocatorJSON = map[string]any{}
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}
	return nil
}

func (m *ParseConfirmationItem) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
