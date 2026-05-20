package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoutePolicyRule struct {
	ID                     uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	PolicyID               uuid.UUID      `gorm:"type:uuid;not null;index:idx_content_parse_policy_rules_policy_order,priority:1;column:policy_id" json:"policy_id"`
	MatchFileTypes         []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"match_file_types,omitempty"`
	MatchMIMEPrefix        string         `gorm:"type:varchar(128);column:match_mime_prefix" json:"match_mime_prefix,omitempty"`
	MatchIsScanned         *bool          `gorm:"column:match_is_scanned" json:"match_is_scanned,omitempty"`
	PreferredProviderOrder []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"preferred_provider_order,omitempty"`
	FallbackProviderOrder  []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"fallback_provider_order,omitempty"`
	RequireLocal           bool           `gorm:"not null;default:false" json:"require_local"`
	AllowVLM               bool           `gorm:"not null;default:true" json:"allow_vlm"`
	MaxTimeoutSec          *int           `gorm:"column:max_timeout_sec" json:"max_timeout_sec,omitempty"`
	SortOrder              int            `gorm:"not null;default:100;index:idx_content_parse_policy_rules_policy_order,priority:2;column:sort_order" json:"sort_order"`
	Metadata               map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"metadata,omitempty"`
	CreatedAt              time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt              time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (RoutePolicyRule) TableName() string {
	return "content_parse_route_policy_rules"
}

func (m *RoutePolicyRule) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
