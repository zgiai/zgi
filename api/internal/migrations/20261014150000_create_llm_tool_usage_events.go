package migrations

import (
	"fmt"
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"time"
)

const migrationCreateLLMToolUsageEventsID = "20261014150000_create_llm_tool_usage_events"

func init() {
	registerMigration(&gormigrate.Migration{
		ID: migrationCreateLLMToolUsageEventsID,
		Migrate: func(db *gorm.DB) error {
			if db.Migrator().HasTable(&toolUsageEventV1{}) {
				return nil
			}
			return db.AutoMigrate(&toolUsageEventV1{})
		},
		Rollback: func(db *gorm.DB) error {
			if !db.Migrator().HasTable(&toolUsageEventV1{}) {
				return nil
			}
			var count int64
			if err := db.Model(&toolUsageEventV1{}).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("tool usage events contain %d audit facts; preserve them and roll back the application only", count)
			}
			return db.Migrator().DropTable(&toolUsageEventV1{})
		},
	})
}

// toolUsageEventV1 freezes the schema at migration creation time.
type toolUsageEventV1 struct {
	SchemaVersion     int            `gorm:"not null;default:1" json:"schema_version"`
	OrganizationID    uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_tool_events_lane_dedupe,priority:1;index" json:"organization_id"`
	ID                uuid.UUID      `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	RequestID         string         `gorm:"column:request_id;size:120;not null;index:idx_tool_events_request" json:"request_id"`
	AttemptID         string         `gorm:"column:attempt_id;size:120;not null;index:idx_tool_events_attempt" json:"attempt_id"`
	EventID           string         `gorm:"column:event_id;size:160;not null" json:"event_id"`
	BillingLane       string         `gorm:"column:billing_lane;size:20;not null;check:chk_tool_events_lane,billing_lane = 'private';index:idx_tool_events_lane_tool_status,priority:1;uniqueIndex:uq_tool_events_lane_dedupe,priority:2" json:"billing_lane"`
	Tool              string         `gorm:"column:tool;size:64;not null;index:idx_tool_events_lane_tool_status,priority:2" json:"tool"`
	ModelID           *uuid.UUID     `gorm:"column:model_id;type:uuid;index" json:"model_id,omitempty"`
	ModelName         string         `gorm:"column:model_name;size:160;index" json:"model_name,omitempty"`
	ProviderName      string         `gorm:"column:provider_name;size:160" json:"provider_name,omitempty"`
	ChannelID         *uuid.UUID     `gorm:"column:channel_id;type:uuid;index" json:"channel_id,omitempty"`
	ProviderRequestID string         `gorm:"column:provider_request_id;size:255;index" json:"provider_request_id,omitempty"`
	ProviderCallID    string         `gorm:"column:provider_call_id;size:255;index" json:"provider_call_id,omitempty"`
	SessionID         string         `gorm:"column:session_id;size:255;index:idx_tool_events_session" json:"session_id,omitempty"`
	ContainerSpec     string         `gorm:"column:container_spec;size:255" json:"container_spec,omitempty"`
	Quantity          int64          `gorm:"column:quantity;not null;default:0" json:"quantity"`
	Unit              string         `gorm:"column:unit;size:32;not null" json:"unit"`
	Status            string         `gorm:"column:status;size:32;not null;index:idx_tool_events_lane_tool_status,priority:3" json:"status"`
	UsageSource       string         `gorm:"column:usage_source;size:64;not null" json:"usage_source"`
	DedupeKey         string         `gorm:"column:dedupe_key;size:512;not null;uniqueIndex:uq_tool_events_lane_dedupe,priority:3" json:"dedupe_key"`
	Details           datatypes.JSON `gorm:"column:details;type:jsonb;not null;default:'{}'" json:"details"`
	FirstObservedAt   time.Time      `gorm:"column:first_observed_at;not null;index" json:"first_observed_at"`
	LastObservedAt    time.Time      `gorm:"column:last_observed_at;not null;index" json:"last_observed_at"`
	CreatedAt         time.Time      `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"column:updated_at;not null" json:"updated_at"`
}

func (toolUsageEventV1) TableName() string { return "llm_tool_usage_events" }
