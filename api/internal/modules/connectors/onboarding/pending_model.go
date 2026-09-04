package onboarding

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type PendingState string

const (
	VerificationRequired PendingState = "verification_required"
	AwaitingApplication  PendingState = "awaiting_application"
	PendingCompleted     PendingState = "completed"
	PendingCancelled     PendingState = "cancelled"
	PendingExpired       PendingState = "expired"
)

func (s PendingState) Active() bool { return s == VerificationRequired || s == AwaitingApplication }

// PendingSetup contains only identity and receipts. It is deliberately not a
// Connection: no candidate credentials, runtime grants, defaults or bindings.
type PendingSetup struct {
	ID                       uuid.UUID `gorm:"primaryKey"`
	OrganizationID           uuid.UUID
	OwnerAccountID           uuid.UUID
	ConnectorID              string
	Region                   string
	AppID                    string
	SourceFlowID             uuid.UUID
	TemplateID               string
	TemplateDigest           string
	AcceptedDefinitionDigest string
	State                    PendingState
	SafeReasonCode           *core.ErrorCode
	LastVerificationAt       *time.Time
	Revision                 int64
	ResultConnectionID       *uuid.UUID
	ResumeExpiresAt          time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
	TerminalAt               *time.Time
}

func (PendingSetup) TableName() string { return "connector_pending_setups" }

type PendingSnapshot struct {
	Kind                      string          `json:"kind"`
	ID                        uuid.UUID       `json:"id"`
	ConnectorID               string          `json:"connector_id"`
	Region                    string          `json:"region"`
	AppID                     string          `json:"app_id"`
	State                     PendingState    `json:"state"`
	Revision                  int64           `json:"revision"`
	ResumeExpiresAt           time.Time       `json:"resume_expires_at"`
	ReasonCode                *core.ErrorCode `json:"reason_code,omitempty"`
	LastVerificationAt        *time.Time      `json:"last_verification_at,omitempty"`
	AllowedActions            []string        `json:"allowed_actions"`
	ResumeQRUnavailableReason string          `json:"resume_qr_unavailable_reason,omitempty"`
	ActiveFlowID              *uuid.UUID      `json:"active_flow_id,omitempty"`
	ResultConnectionID        *uuid.UUID      `json:"result_connection_id,omitempty"`
}
type PendingList struct {
	Items        []PendingSnapshot `json:"items"`
	TotalPending int64             `json:"total_pending"`
}
type PendingRepository interface {
	GetPending(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (PendingSetup, *uuid.UUID, error)
	ListPending(context.Context, uuid.UUID, uuid.UUID, string, int, int) ([]PendingSetup, int64, error)
	CancelPending(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64) (PendingSetup, error)
	ObserveApplication(context.Context, Flow, string) (Flow, error)
	DeferApplication(context.Context, Flow, core.ErrorCode) error
}

func (f Flow) verificationExpiry() time.Time {
	if f.ProtocolVersion == 0 && f.State == Verifying && f.VerificationDeadline != nil {
		return *f.VerificationDeadline
	}
	return f.ExpiresAt
}
