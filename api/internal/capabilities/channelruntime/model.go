package channelruntime

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type InstallationStatus string

const (
	InstallationStatusActive   InstallationStatus = "active"
	InstallationStatusDisabled InstallationStatus = "disabled"
)

// Installation binds one provider application to a workspace. ConnectionID is
// a credential locator: it equals the installation ID for installation-owned
// credentials, or references Connection Center when an existing credential is
// selected. Secret material never crosses into this table.
type Installation struct {
	ID                 uuid.UUID                    `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID     uuid.UUID                    `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID        uuid.UUID                    `gorm:"type:uuid;not null" json:"workspace_id"`
	Provider           string                       `gorm:"type:varchar(32);not null" json:"provider"`
	ConnectionID       uuid.UUID                    `gorm:"type:uuid;not null" json:"connection_id"`
	Name               string                       `gorm:"type:varchar(128);not null" json:"name"`
	ExternalTenantID   string                       `gorm:"type:varchar(255);not null" json:"external_tenant_id,omitempty"`
	ExternalBotID      string                       `gorm:"type:varchar(255);not null" json:"external_bot_id,omitempty"`
	DefaultSubjectType *runtimeidentity.SubjectType `gorm:"type:varchar(32)" json:"default_subject_type,omitempty"`
	DefaultSubjectID   *uuid.UUID                   `gorm:"type:uuid" json:"default_subject_id,omitempty"`
	Status             InstallationStatus           `gorm:"type:varchar(16);not null" json:"status"`
	Revision           int64                        `gorm:"not null" json:"revision"`
	CreatedBy          uuid.UUID                    `gorm:"type:uuid;not null" json:"created_by"`
	UpdatedBy          uuid.UUID                    `gorm:"type:uuid;not null" json:"updated_by"`
	CreatedAt          time.Time                    `gorm:"not null" json:"created_at"`
	UpdatedAt          time.Time                    `gorm:"not null" json:"updated_at"`
	DeletedAt          gorm.DeletedAt               `gorm:"index" json:"-"`
}

func (Installation) TableName() string { return "channel_installations" }

// ConversationMapping maps one provider conversation to one stable local
// ChatRuntime conversation without making that mapping a memory partition.
type ConversationMapping struct {
	ID                     uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID         uuid.UUID        `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID            uuid.UUID        `gorm:"type:uuid;not null" json:"workspace_id"`
	InstallationID         uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_external_conversation_mappings_identity,priority:1" json:"installation_id"`
	ExternalConversationID string           `gorm:"type:varchar(255);not null;uniqueIndex:idx_external_conversation_mappings_identity,priority:2" json:"external_conversation_id"`
	ConversationType       ConversationType `gorm:"type:varchar(16);not null" json:"conversation_type"`
	LocalConversationID    *uuid.UUID       `gorm:"type:uuid" json:"local_conversation_id,omitempty"`
	LastEventAt            *time.Time       `json:"last_event_at,omitempty"`
	CreatedAt              time.Time        `gorm:"not null" json:"created_at"`
	UpdatedAt              time.Time        `gorm:"not null" json:"updated_at"`
}

func (ConversationMapping) TableName() string { return "external_conversation_mappings" }

type WakePolicy string

const (
	WakePolicyDirectAndMention WakePolicy = "direct_and_mention"
	WakePolicyMentionOnly      WakePolicy = "mention_only"
)

// Binding assigns one external conversation to one runtime subject.
type Binding struct {
	ID                    uuid.UUID                   `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID        uuid.UUID                   `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID           uuid.UUID                   `gorm:"type:uuid;not null" json:"workspace_id"`
	InstallationID        uuid.UUID                   `gorm:"type:uuid;not null" json:"installation_id"`
	ConversationMappingID uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex:idx_channel_bindings_conversation" json:"conversation_mapping_id"`
	SubjectType           runtimeidentity.SubjectType `gorm:"type:varchar(32);not null" json:"subject_type"`
	SubjectID             uuid.UUID                   `gorm:"type:uuid;not null" json:"subject_id"`
	WakePolicy            WakePolicy                  `gorm:"type:varchar(32);not null" json:"wake_policy"`
	Enabled               bool                        `gorm:"not null" json:"enabled"`
	Revision              int64                       `gorm:"not null" json:"revision"`
	CreatedBy             uuid.UUID                   `gorm:"type:uuid;not null" json:"created_by"`
	UpdatedBy             uuid.UUID                   `gorm:"type:uuid;not null" json:"updated_by"`
	CreatedAt             time.Time                   `gorm:"not null" json:"created_at"`
	UpdatedAt             time.Time                   `gorm:"not null" json:"updated_at"`
	DeletedAt             gorm.DeletedAt              `gorm:"index" json:"-"`
}

func (Binding) TableName() string { return "channel_bindings" }

type InboundEventStatus string

const (
	InboundEventReceived InboundEventStatus = "received"
	InboundEventIgnored  InboundEventStatus = "ignored"
	InboundEventAccepted InboundEventStatus = "accepted"
	InboundEventFailed   InboundEventStatus = "failed"
)

// InboundEvent is the deduplicated audit record for one provider callback.
// It stores a bounded display preview of the triggering message, identifiers,
// and a digest, but never the raw callback, fetched channel history, or secrets.
type InboundEvent struct {
	ID                     uuid.UUID          `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID         uuid.UUID          `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID            uuid.UUID          `gorm:"type:uuid;not null" json:"workspace_id"`
	InstallationID         uuid.UUID          `gorm:"type:uuid;not null;uniqueIndex:idx_channel_inbound_events_provider_event,priority:1;uniqueIndex:idx_channel_inbound_events_provider_message,priority:1,where:provider_message_id <> ''" json:"installation_id"`
	BindingID              *uuid.UUID         `gorm:"type:uuid" json:"binding_id,omitempty"`
	ProviderEventID        string             `gorm:"type:varchar(255);not null;uniqueIndex:idx_channel_inbound_events_provider_event,priority:2" json:"provider_event_id"`
	ProviderMessageID      string             `gorm:"type:varchar(255);not null;uniqueIndex:idx_channel_inbound_events_provider_message,priority:2,where:provider_message_id <> ''" json:"provider_message_id"`
	ExternalConversationID string             `gorm:"type:varchar(255);not null" json:"external_conversation_id"`
	SenderExternalID       string             `gorm:"type:varchar(255);not null" json:"sender_external_id"`
	ConversationType       ConversationType   `gorm:"type:varchar(16);not null" json:"conversation_type"`
	ContentPreview         string             `gorm:"type:varchar(512);not null" json:"content_preview"`
	MentionOnly            bool               `gorm:"not null" json:"mention_only"`
	EventType              string             `gorm:"type:varchar(64);not null" json:"event_type"`
	PayloadDigest          string             `gorm:"type:varchar(71);not null" json:"payload_digest"`
	Status                 InboundEventStatus `gorm:"type:varchar(16);not null" json:"status"`
	ReasonCode             string             `gorm:"type:varchar(128);not null" json:"reason_code,omitempty"`
	InvocationRunID        *uuid.UUID         `gorm:"type:uuid" json:"invocation_run_id,omitempty"`
	ReceivedAt             time.Time          `gorm:"not null" json:"received_at"`
	ProcessedAt            *time.Time         `json:"processed_at,omitempty"`
	CreatedAt              time.Time          `gorm:"not null" json:"created_at"`
	UpdatedAt              time.Time          `gorm:"not null" json:"updated_at"`
}

func (InboundEvent) TableName() string { return "channel_inbound_events" }

type DeliveryStatus string

const (
	DeliveryPending    DeliveryStatus = "pending"
	DeliveryProcessing DeliveryStatus = "processing"
	DeliverySucceeded  DeliveryStatus = "succeeded"
	DeliveryFailed     DeliveryStatus = "failed"
	DeliveryUnknown    DeliveryStatus = "unknown"
)

type DeliveryKind string

const (
	DeliveryKindAcknowledgement DeliveryKind = "acknowledgement"
	DeliveryKindProgress        DeliveryKind = "progress"
	DeliveryKindFinal           DeliveryKind = "final"
)

// Delivery is the durable channel send intent. Payload contains bounded public
// message content only; credentials are resolved from the installation's
// credential locator at execution.
type Delivery struct {
	ID                     uuid.UUID                   `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID         uuid.UUID                   `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID            uuid.UUID                   `gorm:"type:uuid;not null" json:"workspace_id"`
	InstallationID         uuid.UUID                   `gorm:"type:uuid;not null" json:"installation_id"`
	BindingID              uuid.UUID                   `gorm:"type:uuid;not null" json:"binding_id"`
	InboundEventID         uuid.UUID                   `gorm:"type:uuid;not null" json:"inbound_event_id"`
	RunID                  uuid.UUID                   `gorm:"type:uuid;not null;index:idx_channel_delivery_outbox_run" json:"run_id"`
	Kind                   DeliveryKind                `gorm:"column:delivery_kind;type:varchar(24);not null;default:final" json:"kind"`
	DeliveryKey            string                      `gorm:"type:varchar(128);not null;default:final" json:"delivery_key"`
	Sequence               int64                       `gorm:"not null;default:9223372036854775807" json:"sequence"`
	Provider               string                      `gorm:"type:varchar(32);not null" json:"provider"`
	ConnectionID           uuid.UUID                   `gorm:"type:uuid;not null" json:"connection_id"`
	ActorID                uuid.UUID                   `gorm:"type:uuid;not null" json:"actor_id"`
	SubjectType            runtimeidentity.SubjectType `gorm:"type:varchar(32);not null" json:"subject_type"`
	SubjectID              uuid.UUID                   `gorm:"type:uuid;not null" json:"subject_id"`
	ExternalConversationID string                      `gorm:"type:varchar(255);not null" json:"external_conversation_id"`
	ExternalThreadID       string                      `gorm:"type:varchar(255);not null" json:"external_thread_id,omitempty"`
	Payload                datatypes.JSON              `gorm:"type:jsonb;not null" json:"payload"`
	PayloadDigest          string                      `gorm:"type:varchar(71);not null" json:"payload_digest"`
	Status                 DeliveryStatus              `gorm:"type:varchar(16);not null" json:"status"`
	AttemptCount           int                         `gorm:"not null" json:"attempt_count"`
	AvailableAt            time.Time                   `gorm:"not null" json:"available_at"`
	ClaimedAt              *time.Time                  `json:"claimed_at,omitempty"`
	DeliveredAt            *time.Time                  `json:"delivered_at,omitempty"`
	ProviderMessageID      string                      `gorm:"type:varchar(255);not null" json:"provider_message_id,omitempty"`
	ProviderRequestID      string                      `gorm:"type:varchar(128);not null" json:"provider_request_id,omitempty"`
	ErrorCode              string                      `gorm:"type:varchar(128);not null" json:"error_code,omitempty"`
	ErrorMessage           string                      `gorm:"type:text;not null" json:"error_message,omitempty"`
	CreatedAt              time.Time                   `gorm:"not null" json:"created_at"`
	UpdatedAt              time.Time                   `gorm:"not null" json:"updated_at"`
}

func (Delivery) TableName() string { return "channel_delivery_outbox" }
