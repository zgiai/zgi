package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProviderHealthCheck struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	ProviderConfigID uuid.UUID      `gorm:"type:uuid;not null;index:idx_content_parse_health_provider_checked,priority:1;column:provider_config_id" json:"provider_config_id"`
	Status           string         `gorm:"type:varchar(32);not null;index:idx_content_parse_health_status_checked,priority:1" json:"status"`
	LatencyMS        *int           `gorm:"column:latency_ms" json:"latency_ms,omitempty"`
	ErrorMessage     string         `gorm:"type:text;column:error_message" json:"error_message,omitempty"`
	Details          map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"details,omitempty"`
	CheckedAt        time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_content_parse_health_provider_checked,priority:2;index:idx_content_parse_health_status_checked,priority:2;column:checked_at" json:"checked_at"`
}

func (ProviderHealthCheck) TableName() string {
	return "content_parse_provider_health_checks"
}

func (m *ProviderHealthCheck) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.CheckedAt.IsZero() {
		m.CheckedAt = time.Now()
	}
	return nil
}
