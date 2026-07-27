package integrations

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ConnectionHealthSource string

const (
	ConnectionHealthSourceManual       ConnectionHealthSource = "manual"
	ConnectionHealthSourceRuntime      ConnectionHealthSource = "runtime"
	ConnectionHealthSourceOAuthRefresh ConnectionHealthSource = "oauth_refresh"
)

type ConnectionHealthCheckKind string

const (
	ConnectionHealthCheckFull    ConnectionHealthCheckKind = "full"
	ConnectionHealthCheckAuth    ConnectionHealthCheckKind = "auth"
	ConnectionHealthCheckScope   ConnectionHealthCheckKind = "scope"
	ConnectionHealthCheckPassive ConnectionHealthCheckKind = "passive"
)

type ConnectionHealthClassification string

const (
	ConnectionHealthClassificationSuccess          ConnectionHealthClassification = "success"
	ConnectionHealthClassificationAuthInvalid      ConnectionHealthClassification = "auth_invalid"
	ConnectionHealthClassificationOAuthExpired     ConnectionHealthClassification = "oauth_expired"
	ConnectionHealthClassificationScopeDrift       ConnectionHealthClassification = "scope_drift"
	ConnectionHealthClassificationAccessDenied     ConnectionHealthClassification = "access_denied"
	ConnectionHealthClassificationBudgetExhausted  ConnectionHealthClassification = "budget_exhausted"
	ConnectionHealthClassificationRateLimited      ConnectionHealthClassification = "rate_limited"
	ConnectionHealthClassificationTransient        ConnectionHealthClassification = "transient"
	ConnectionHealthClassificationProviderIncident ConnectionHealthClassification = "provider_incident"
	ConnectionHealthClassificationIgnored          ConnectionHealthClassification = "ignored"
)

type ConnectionHealthEvent struct {
	ID                 uuid.UUID                      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID     uuid.UUID                      `gorm:"type:uuid;not null;index:idx_connection_health_events_org_observed,priority:1" json:"organization_id"`
	ConnectionID       uuid.UUID                      `gorm:"type:uuid;not null;index:idx_connection_health_events_connection_observed,priority:1" json:"connection_id"`
	IntegrationID      string                         `gorm:"size:64;not null" json:"integration_id"`
	DriverID           string                         `gorm:"size:64;not null" json:"driver_id"`
	Source             ConnectionHealthSource         `gorm:"size:32;not null" json:"source"`
	CheckKind          ConnectionHealthCheckKind      `gorm:"size:32;not null" json:"check_kind"`
	Classification     ConnectionHealthClassification `gorm:"size:64;not null" json:"classification"`
	ReasonCode         *string                        `gorm:"size:64" json:"reason_code,omitempty"`
	HealthStatusAfter  ConnectionHealthStatus         `gorm:"size:32;not null" json:"health_status_after"`
	AuthStatusAfter    ConnectionAuthStatus           `gorm:"size:32;not null" json:"auth_status_after"`
	ScopeStatusAfter   ConnectionScopeStatus          `gorm:"size:32;not null" json:"scope_status_after"`
	AttentionCodeAfter *string                        `gorm:"size:64" json:"attention_code_after,omitempty"`
	CredentialVersion  int                            `gorm:"not null" json:"credential_version"`
	HealthRevision     int                            `gorm:"not null" json:"health_revision"`
	ExecutionID        *uuid.UUID                     `gorm:"type:uuid;index" json:"execution_id,omitempty"`
	ActorID            *uuid.UUID                     `gorm:"type:uuid" json:"actor_id,omitempty"`
	ProviderRequestID  *string                        `gorm:"size:128" json:"provider_request_id,omitempty"`
	ProviderHTTPStatus *int                           `json:"provider_http_status,omitempty"`
	LatencyMS          int64                          `gorm:"not null;default:0" json:"latency_ms"`
	RetryAfterAt       *time.Time                     `json:"retry_after_at,omitempty"`
	GrantedScopes      []string                       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"granted_scopes,omitempty"`
	AddedScopes        []string                       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"added_scopes,omitempty"`
	RemovedScopes      []string                       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"removed_scopes,omitempty"`
	MissingScopes      []string                       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"missing_scopes,omitempty"`
	ErrorFingerprint   *string                        `gorm:"size:64" json:"-"`
	Applied            bool                           `gorm:"not null;default:false" json:"applied"`
	ObservedAt         time.Time                      `gorm:"not null" json:"observed_at"`
	CreatedAt          time.Time                      `json:"created_at"`
}

func (ConnectionHealthEvent) TableName() string { return "integration_connection_health_events" }

type ConnectionHealthObservation struct {
	OrganizationID         uuid.UUID
	ConnectionID           uuid.UUID
	IntegrationID          string
	DriverID               string
	Source                 ConnectionHealthSource
	CheckKind              ConnectionHealthCheckKind
	Classification         ConnectionHealthClassification
	ReasonCode             string
	CredentialVersion      int
	ExpectedHealthRevision int
	ExecutionID            *uuid.UUID
	ActorID                *uuid.UUID
	ProviderRequestID      string
	ProviderHTTPStatus     *int
	LatencyMS              int64
	RetryAfterAt           *time.Time
	GrantedScopes          []string
	ScopeSnapshotObserved  bool
	MissingScopes          []string
	ErrorFingerprint       string
	ObservedAt             time.Time
	FailureThreshold       int
	// SummaryAlreadyApplied records a manual-test event after the management
	// transaction has already persisted the same health outcome. It prevents
	// the event writer from incrementing counters a second time.
	SummaryAlreadyApplied bool
}

type ConnectionHealthSignal struct {
	OrganizationID    uuid.UUID
	ConnectionID      uuid.UUID
	IntegrationID     string
	DriverID          string
	ActionID          string
	CredentialVersion int
	ExecutionID       uuid.UUID
	ProviderRequestID string
	DurationMS        int64
	ErrorCode         string
	ObservedAt        time.Time
}

type ConnectionHealthSignalSink interface {
	PublishConnectionHealthSignal(ctx context.Context, signal ConnectionHealthSignal) error
}

type ConnectionHealthObservationRecorder interface {
	RecordConnectionHealthObservation(ctx context.Context, observation ConnectionHealthObservation) (ConnectionHealthEvent, error)
}
