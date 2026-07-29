package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	GraphOutboxStatusPending    = "pending"
	GraphOutboxStatusProcessing = "processing"
	GraphOutboxStatusConfirmed  = "confirmed"
	GraphOutboxStatusFailed     = "failed"
)

const (
	GraphOutboxEventRun          = "graph_run"
	GraphOutboxEventVisibility   = "graph_visibility"
	GraphOutboxEventCleanup      = "graph_cleanup"
	GraphOutboxEventDatasetPurge = "dataset_purge"
)

type GraphOutboxEvent struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;index" json:"organization_id"`
	WorkspaceID    *uuid.UUID     `gorm:"type:uuid;index" json:"workspace_id,omitempty"`
	DatasetID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"dataset_id"`
	RunID          *uuid.UUID     `gorm:"type:uuid;index" json:"run_id,omitempty"`
	EventType      string         `gorm:"type:varchar(64);not null;index:idx_graph_outbox_active_aggregate,priority:1" json:"event_type"`
	AggregateKey   string         `gorm:"type:varchar(512);not null;index:idx_graph_outbox_active_aggregate,priority:2" json:"aggregate_key"`
	Payload        map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"payload"`
	Status         string         `gorm:"type:varchar(32);not null;default:'pending';index:idx_graph_outbox_claim,priority:1" json:"status"`
	AttemptCount   int            `gorm:"not null;default:0" json:"attempt_count"`
	AvailableAt    time.Time      `gorm:"not null;index:idx_graph_outbox_claim,priority:2" json:"available_at"`
	ClaimedAt      *time.Time     `json:"claimed_at,omitempty"`
	ConfirmedAt    *time.Time     `json:"confirmed_at,omitempty"`
	ErrorMessage   *string        `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt      time.Time      `gorm:"autoCreateTime;index:idx_graph_outbox_claim,priority:3" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (GraphOutboxEvent) TableName() string {
	return "graph_outbox_events"
}

func (m *GraphOutboxEvent) BeforeCreate(_ *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = GraphOutboxStatusPending
	}
	if m.Payload == nil {
		m.Payload = map[string]any{}
	}
	if m.AvailableAt.IsZero() {
		m.AvailableAt = time.Now().UTC()
	}
	return nil
}
