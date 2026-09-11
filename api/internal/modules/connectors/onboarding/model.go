package onboarding

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"time"
)

type State string

const (
	Starting            State = "starting"
	Awaiting            State = "awaiting_authorization"
	Verifying           State = "verifying"
	VerificationWaiting State = "verification_waiting"
	Ready               State = "ready"
	Cancelled           State = "cancelled"
	Expired             State = "expired"
	Failed              State = "failed"
	RecoveryRequired    State = "recovery_required"
	PendingCompletion   State = "pending_completion"
)

func (s State) Active() bool {
	return s == Starting || s == Awaiting || s == Verifying || s == VerificationWaiting
}

var activeStates = []State{Starting, Awaiting, Verifying, VerificationWaiting}

// Flow is a private durable record. Transport must use Snapshot, never Flow.
type Flow struct {
	ID                         uuid.UUID `gorm:"primaryKey"`
	OrganizationID             uuid.UUID
	OwnerAccountID             uuid.UUID
	ConnectorID                string
	Intent                     string
	PendingSetupID             *uuid.UUID
	ExpectedPendingRevision    *int64
	ManualVerification         bool
	VerificationDeadline       *time.Time
	CandidateConnectionID      *uuid.UUID
	CandidateAcceptedAt        *time.Time
	CandidateExpiresAt         *time.Time
	VerificationPhase          string
	DiagnosticTraceID          string
	RetryKeyHash               *string `json:"-"`
	RetryExpectedRevision      *int64  `json:"-"`
	ConnectionID               *uuid.UUID
	ExpectedConnectionRevision *int
	ExpectedCredentialVersion  *int
	ExpectedAppID              *string
	ExpectedIdentity           *string
	ProtocolVersion            int
	Handler                    string
	Step                       string
	StepDispatched             bool
	ChallengeRevision          int64
	ChallengeAttempts          int
	ExpirySource               string
	Region                     string
	RequestedRegion            string
	TemplateID                 string
	TemplateDigest             string
	AcceptedDefinitionDigest   string
	IdempotencyKeyHash         string `json:"-"`
	RequestFingerprint         string `json:"-"`
	State                      State
	SafeErrorCode              *core.ErrorCode
	ResultConnectionID         *uuid.UUID
	Generation                 int
	Revision                   int64
	LeaseOwner                 *uuid.UUID `json:"-"`
	LeaseUntil                 *time.Time `json:"-"`
	Fencing                    int64      `json:"-"`
	BeginDispatched            bool
	SwitchedRegion             bool
	PollIntervalSeconds        int
	ExpiresAt                  time.Time
	NextPollAt                 time.Time
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	TerminalAt                 *time.Time
}

func (Flow) TableName() string { return "connector_onboarding_flows" }

type DeviceRecord struct {
	FlowID          uuid.UUID `gorm:"primaryKey"`
	OrganizationID  uuid.UUID
	OwnerAccountID  uuid.UUID
	Generation      int
	EncryptedDevice string `json:"-"`
	ExpiresAt       time.Time
}

func (DeviceRecord) TableName() string { return "connector_onboarding_secrets" }

type Snapshot struct {
	CandidateExpiresAt   *time.Time      `json:"candidate_expires_at,omitempty"`
	VerificationDeadline *time.Time      `json:"verification_deadline,omitempty"`
	VerificationPhase    string          `json:"verification_phase,omitempty"`
	DiagnosticTraceID    string          `json:"diagnostic_trace_id,omitempty"`
	ProtocolVersion      int             `json:"protocol_version,omitempty"`
	Step                 string          `json:"step,omitempty"`
	ExpirySource         string          `json:"expiry_source,omitempty"`
	QRPayload            string          `json:"qr_payload,omitempty"`
	AllowedActions       []string        `json:"allowed_actions,omitempty"`
	ChallengeRevision    int64           `json:"challenge_revision,omitempty"`
	ID                   uuid.UUID       `json:"id"`
	ConnectorID          string          `json:"connector_id"`
	Intent               string          `json:"intent"`
	State                State           `json:"state"`
	Region               string          `json:"region"`
	Revision             int64           `json:"revision"`
	ExpiresAt            time.Time       `json:"expires_at"`
	RetryAfter           int             `json:"retry_after"`
	ErrorCode            *core.ErrorCode `json:"error_code,omitempty"`
	VerificationURL      string          `json:"verification_url,omitempty"`
	UserCode             string          `json:"user_code,omitempty"`
	ResultConnectionID   *uuid.UUID      `json:"result_connection_id,omitempty"`
	PendingSetupID       *uuid.UUID      `json:"pending_setup_id,omitempty"`
	QRExpiresAt          *time.Time      `json:"qr_expires_at,omitempty"`
}
type Repository interface {
	FindRequest(context.Context, uuid.UUID, uuid.UUID, string) (Flow, bool, error)
	Admit(context.Context, Flow) (Flow, error)
	Get(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (Flow, error)
	Cancel(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64) (Flow, error)
	Claim(context.Context, uuid.UUID) (Flow, bool, error)
	Device(context.Context, Flow) (DeviceRecord, error)
	AwaitAuthorization(context.Context, Flow, DeviceRecord, time.Duration, time.Duration) error
	Advance(context.Context, Flow, string, bool, int) error
	Terminate(context.Context, Flow, State, core.ErrorCode) error
	Complete(context.Context, Flow, map[string]string) (uuid.UUID, error)
}

// Authority rechecks current membership and accepted plugin identity before
// decrypting a device session, making an external request and committing it.
type Authority interface {
	Check(context.Context, Flow) error
}
