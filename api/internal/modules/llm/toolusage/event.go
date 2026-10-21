package toolusage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ownedLane = "private"

const (
	LanePlatform = "platform"
	LanePrivate  = "private"

	ToolWebSearch       = "web_search"
	ToolFileSearch      = "file_search"
	ToolCodeInterpreter = "code_interpreter"

	StatusObserved  = "observed"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusPending   = "pending"

	UsageSourceProviderReported = "provider_reported"
	UsageSourceGatewayCounted   = "gateway_counted"
	UsageSourceMissing          = "missing"
)

var (
	ErrInvalidEvent = errors.New("invalid tool usage event")
	ErrInvalidLane  = errors.New("invalid billing lane")
	ErrFactConflict = errors.New("conflicting confirmed tool usage fact")
)

// Event is the durable tool usage fact shared by the platform and private lanes.
// It deliberately contains no price or settlement fields; T4 owns those concerns.
type Event struct {
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
	UpstreamScope     string         `gorm:"column:upstream_scope;size:128;not null;default:''" json:"upstream_scope,omitempty"`
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

func (Event) TableName() string { return "llm_tool_usage_events" }

func (e *Event) BeforeCreate(_ *gorm.DB) error {
	if e.SchemaVersion == 0 {
		e.SchemaVersion = 1
	}
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.FirstObservedAt.IsZero() {
		e.FirstObservedAt = time.Now().UTC()
	}
	if e.LastObservedAt.IsZero() {
		e.LastObservedAt = e.FirstObservedAt
	}
	if len(e.Details) == 0 {
		e.Details = datatypes.JSON([]byte(`{}`))
	}
	return e.Validate()
}

func (e *Event) Validate() error {
	if e == nil || e.OrganizationID == uuid.Nil || (e.SchemaVersion != 0 && e.SchemaVersion != 1) || strings.TrimSpace(e.RequestID) == "" || strings.TrimSpace(e.AttemptID) == "" || strings.TrimSpace(e.EventID) == "" || strings.TrimSpace(e.DedupeKey) == "" {
		return ErrInvalidEvent
	}
	if e.BillingLane != LanePlatform && e.BillingLane != LanePrivate {
		return ErrInvalidLane
	}
	switch e.Tool {
	case ToolWebSearch, ToolFileSearch, ToolCodeInterpreter:
	default:
		return fmt.Errorf("%w: unsupported tool", ErrInvalidEvent)
	}
	if e.Quantity < 0 {
		return ErrInvalidEvent
	}
	if e.Tool == ToolCodeInterpreter {
		if e.Unit != "session" {
			return ErrInvalidEvent
		}
		if e.Status == StatusSucceeded && (e.SessionID == "" || e.Quantity != 1) {
			return ErrInvalidEvent
		}
	} else if e.Unit != "call" {
		return ErrInvalidEvent
	}
	switch e.Status {
	case StatusObserved, StatusPending, StatusSucceeded, StatusFailed:
	default:
		return ErrInvalidEvent
	}
	switch e.UsageSource {
	case UsageSourceProviderReported, UsageSourceGatewayCounted, UsageSourceMissing:
	default:
		return ErrInvalidEvent
	}
	if (e.Status == StatusFailed || e.UsageSource == UsageSourceMissing || e.Status == StatusPending || e.Status == StatusObserved) && e.Quantity != 0 {
		return ErrInvalidEvent
	}
	if e.Status == StatusSucceeded && e.UsageSource == UsageSourceMissing {
		return ErrInvalidEvent
	}
	if len(e.Details) != 0 && !json.Valid(e.Details) {
		return ErrInvalidEvent
	}

	return nil
}

func (e *Event) SetDetails(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	e.Details = datatypes.JSON(encoded)
	return nil
}

// Repository records synchronous provider facts idempotently.
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Record(ctx context.Context, event *Event) (*Event, error) {
	if event == nil || event.Tool != ToolCodeInterpreter || event.Status != StatusSucceeded {
		return r.record(ctx, event)
	}
	if r == nil || r.db == nil {
		return nil, errors.New("tool usage repository is nil")
	}
	if event.BillingLane != ownedLane {
		return nil, ErrInvalidLane
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	var stored *Event
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize reuse only within the same selected route and upstream account.
		if tx.Dialector.Name() == "postgres" {
			identity := event.OrganizationID.String() + ":" + event.BillingLane + ":" + event.ProviderName + ":" + event.ChannelID.String() + ":" + event.UpstreamScope + ":" + event.SessionID
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", identity).Error; err != nil {
				return err
			}
		}
		var existing Event
		err := tx.Where("organization_id = ? AND billing_lane = ? AND provider_name = ? AND channel_id = ? AND upstream_scope = ? AND session_id = ? AND status = ?", event.OrganizationID, event.BillingLane, event.ProviderName, event.ChannelID, event.UpstreamScope, event.SessionID, StatusSucceeded).Take(&existing).Error
		if err == nil {
			stored = &existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		stored, err = (&Repository{db: tx}).record(ctx, event)
		return err
	})
	return stored, err
}

func (r *Repository) record(ctx context.Context, event *Event) (*Event, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("tool usage repository is nil")
	}
	if event == nil {
		return nil, ErrInvalidEvent
	}
	if event.BillingLane != "private" {
		return nil, ErrInvalidLane
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if event.FirstObservedAt.IsZero() {
		event.FirstObservedAt = now
	}
	event.LastObservedAt = now
	if len(event.Details) == 0 {
		event.Details = datatypes.JSON([]byte(`{}`))
	}
	tx := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "organization_id"}, {Name: "billing_lane"}, {Name: "dedupe_key"}},
		Where:   clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "(llm_tool_usage_events.status IN (?, ?) OR llm_tool_usage_events.usage_source = 'missing') AND excluded.status IN (?, ?)", Vars: []any{StatusPending, StatusObserved, StatusSucceeded, StatusFailed}}}},
		DoUpdates: clause.Assignments(map[string]any{
			"quantity": event.Quantity, "unit": event.Unit, "status": event.Status,
			"usage_source": event.UsageSource, "details": event.Details,
			"provider_request_id": event.ProviderRequestID, "provider_call_id": event.ProviderCallID,
			"session_id": event.SessionID, "container_spec": event.ContainerSpec,
			"last_observed_at": event.LastObservedAt, "updated_at": event.LastObservedAt,
		}),
	}).Create(event)
	if tx.Error != nil {
		return nil, tx.Error
	}
	var stored Event
	if err := r.db.WithContext(ctx).Where("organization_id = ? AND billing_lane = ? AND dedupe_key = ?", event.OrganizationID, event.BillingLane, event.DedupeKey).First(&stored).Error; err != nil {
		return nil, err
	}
	if (stored.Status == StatusSucceeded || stored.Status == StatusFailed) && (event.Status == StatusSucceeded || event.Status == StatusFailed) && (stored.Quantity != event.Quantity || stored.Status != event.Status || stored.Unit != event.Unit) {
		return nil, ErrFactConflict
	}
	return &stored, nil
}

func (r *Repository) ListByAttempt(ctx context.Context, organizationID uuid.UUID, lane, attemptID string) ([]Event, error) {
	if lane != "private" {
		return nil, ErrInvalidLane
	}
	var events []Event
	err := r.db.WithContext(ctx).Where("organization_id = ? AND billing_lane = ? AND attempt_id = ?", organizationID, lane, attemptID).Order("first_observed_at ASC").Find(&events).Error
	return events, err
}

// DedupeKey is based only on trusted upstream identity, never client tool definitions.
// Session identities exclude request/attempt so reuse cannot create another fact.
func DedupeKey(provider, channel, upstreamScope, tool, providerRequestID, providerCallID, sessionID string) (string, error) {
	var identity []string
	if provider == "" || channel == "" {
		return "", ErrInvalidEvent
	}
	switch tool {
	case ToolCodeInterpreter:
		if sessionID == "" {
			return "", ErrInvalidEvent
		}
		identity = []string{provider, channel, upstreamScope, tool, "session", sessionID}
	case ToolWebSearch, ToolFileSearch:
		if providerRequestID == "" {
			return "", ErrInvalidEvent
		}
		identity = []string{provider, channel, upstreamScope, tool, "call", providerRequestID, providerCallID}
	default:
		return "", ErrInvalidEvent
	}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
