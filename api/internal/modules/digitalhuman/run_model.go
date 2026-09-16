package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type RunStatus string

const (
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusWaiting   RunStatus = "waiting"
	RunStatusCanceling RunStatus = "canceling"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCanceled  RunStatus = "canceled"
	RunStatusUnknown   RunStatus = "unknown"
)

type Run struct {
	ID                        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID            uuid.UUID      `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID               uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
	DigitalHumanID            uuid.UUID      `gorm:"type:uuid;not null" json:"digital_human_id"`
	DigitalHumanVersionID     uuid.UUID      `gorm:"type:uuid;not null" json:"digital_human_version_id"`
	ActorID                   *uuid.UUID     `gorm:"type:uuid" json:"actor_id,omitempty"`
	PrincipalType             string         `gorm:"type:varchar(32);not null;default:'';column:principal_type" json:"principal_type,omitempty"`
	PrincipalID               *uuid.UUID     `gorm:"type:uuid;column:principal_id" json:"principal_id,omitempty"`
	UserSubjectKind           string         `gorm:"type:varchar(32);not null;default:'';column:user_subject_kind" json:"user_subject_kind,omitempty"`
	UserSubjectID             *uuid.UUID     `gorm:"type:uuid;column:user_subject_id" json:"user_subject_id,omitempty"`
	Surface                   string         `gorm:"type:varchar(32);not null;default:'digital_human_console';column:surface" json:"surface"`
	RequestFingerprint        string         `gorm:"type:varchar(71);not null;default:'';column:request_fingerprint" json:"-"`
	IdempotencyKey            string         `gorm:"type:varchar(160);not null" json:"idempotency_key"`
	InvocationEvent           datatypes.JSON `gorm:"type:jsonb;not null" json:"invocation_event"`
	RunSpec                   datatypes.JSON `gorm:"type:jsonb;not null" json:"run_spec"`
	RunSpecRevision           string         `gorm:"type:varchar(71);not null" json:"run_spec_revision"`
	ExecutionSnapshot         datatypes.JSON `gorm:"type:jsonb;not null" json:"execution_snapshot"`
	ExecutionSnapshotRevision string         `gorm:"type:varchar(71);not null" json:"execution_snapshot_revision"`
	ExecutionTrace            datatypes.JSON `gorm:"type:jsonb;not null" json:"execution_trace"`
	Query                     string         `gorm:"type:text;not null" json:"query"`
	Status                    RunStatus      `gorm:"type:varchar(16);not null" json:"status"`
	WaitingReason             string         `gorm:"type:varchar(32);not null;default:''" json:"waiting_reason,omitempty"`
	WaitingReceipt            *uuid.UUID     `gorm:"type:uuid" json:"waiting_receipt,omitempty"`
	ConversationID            *uuid.UUID     `gorm:"type:uuid" json:"conversation_id,omitempty"`
	MessageID                 *uuid.UUID     `gorm:"type:uuid" json:"message_id,omitempty"`
	ResultSummary             string         `gorm:"type:text;not null" json:"result_summary,omitempty"`
	ErrorCode                 string         `gorm:"type:varchar(128);not null" json:"error_code,omitempty"`
	ErrorMessage              string         `gorm:"type:text;not null" json:"error_message,omitempty"`
	CancelRequestedAt         *time.Time     `json:"cancel_requested_at,omitempty"`
	StartedAt                 *time.Time     `json:"started_at,omitempty"`
	CompletedAt               *time.Time     `json:"completed_at,omitempty"`
	CreatedAt                 time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt                 time.Time      `gorm:"not null" json:"updated_at"`
}

func (Run) TableName() string { return "digital_human_runs" }

type RunOutboxStatus string

const (
	RunOutboxPending    RunOutboxStatus = "pending"
	RunOutboxProcessing RunOutboxStatus = "processing"
	RunOutboxConfirmed  RunOutboxStatus = "confirmed"
	RunOutboxFailed     RunOutboxStatus = "failed"
)

type RunOutbox struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID       `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID    uuid.UUID       `gorm:"type:uuid;not null" json:"workspace_id"`
	RunID          uuid.UUID       `gorm:"type:uuid;not null" json:"run_id"`
	Status         RunOutboxStatus `gorm:"type:varchar(16);not null" json:"status"`
	AttemptCount   int             `gorm:"not null" json:"attempt_count"`
	AvailableAt    time.Time       `gorm:"not null" json:"available_at"`
	ClaimedAt      *time.Time      `json:"claimed_at,omitempty"`
	ConfirmedAt    *time.Time      `json:"confirmed_at,omitempty"`
	ErrorMessage   string          `gorm:"type:text;not null" json:"error_message,omitempty"`
	CreatedAt      time.Time       `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time       `gorm:"not null" json:"updated_at"`
}

func (RunOutbox) TableName() string { return "digital_human_run_outbox" }
