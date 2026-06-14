package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	RiskLevelLow      = "low"
	RiskLevelMedium   = "medium"
	RiskLevelHigh     = "high"
	RiskLevelCritical = "critical"

	ActionRunStatusPlanned           = "planned"
	ActionRunStatusNeedsConfirmation = "needs_confirmation"
	ActionRunStatusConfirmed         = "confirmed"
	ActionRunStatusCanceled          = "canceled"
	ActionRunStatusRunning           = "running"
	ActionRunStatusBlocked           = "blocked"
	ActionRunStatusCompleted         = "completed"
	ActionRunStatusFailed            = "failed"

	ActionStepStatusPending = "pending"
	ActionStepStatusRunning = "running"
	ActionStepStatusBlocked = "blocked"
	ActionStepStatusDone    = "done"
	ActionStepStatusFailed  = "failed"
)

// ActionRun records the product-level control-plane state for a user requested
// operation. The execution remains delegated to feature adapters.
type ActionRun struct {
	ID                   uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID       uuid.UUID              `gorm:"type:uuid;not null;index:idx_action_runtime_runs_owner_created,priority:1" json:"organization_id"`
	WorkspaceID          *uuid.UUID             `gorm:"type:uuid;index:idx_action_runtime_runs_workspace" json:"workspace_id,omitempty"`
	AccountID            uuid.UUID              `gorm:"type:uuid;not null;index:idx_action_runtime_runs_owner_created,priority:2" json:"account_id"`
	ConversationID       *uuid.UUID             `gorm:"type:uuid;index:idx_action_runtime_runs_conversation" json:"conversation_id,omitempty"`
	MessageID            *uuid.UUID             `gorm:"type:uuid;index:idx_action_runtime_runs_message" json:"message_id,omitempty"`
	IdempotencyKey       *string                `gorm:"type:varchar(128)" json:"idempotency_key,omitempty"`
	Intent               string                 `gorm:"type:varchar(128);not null;default:''" json:"intent"`
	CapabilityID         string                 `gorm:"type:varchar(128);not null;index:idx_action_runtime_runs_capability" json:"capability_id"`
	Title                string                 `gorm:"type:varchar(255);not null;default:''" json:"title"`
	Summary              string                 `gorm:"type:text;not null;default:''" json:"summary"`
	Status               string                 `gorm:"type:varchar(32);not null;default:'planned';index:idx_action_runtime_runs_status" json:"status"`
	RiskLevel            string                 `gorm:"type:varchar(32);not null;default:'low'" json:"risk_level"`
	RequiresConfirmation bool                   `gorm:"not null;default:false" json:"requires_confirmation"`
	ConfirmedBy          *uuid.UUID             `gorm:"type:uuid" json:"confirmed_by,omitempty"`
	ConfirmedAt          *time.Time             `json:"confirmed_at,omitempty"`
	CanceledAt           *time.Time             `json:"canceled_at,omitempty"`
	Error                *string                `gorm:"type:text" json:"error,omitempty"`
	Resources            map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"resources,omitempty"`
	Arguments            map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"arguments,omitempty"`
	Ledger               map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"ledger,omitempty"`
	Metadata             map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"metadata,omitempty"`
	CreatedAt            time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_action_runtime_runs_owner_created,priority:3" json:"created_at"`
	UpdatedAt            time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt            *time.Time             `gorm:"index" json:"deleted_at,omitempty"`
}

func (ActionRun) TableName() string {
	return "action_runtime_runs"
}

// ActionStep records each planned control-plane step in an action run.
type ActionStep struct {
	ID                   uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	RunID                uuid.UUID              `gorm:"type:uuid;not null;index:idx_action_runtime_steps_run_index,priority:1" json:"run_id"`
	StepIndex            int                    `gorm:"not null;default:0;index:idx_action_runtime_steps_run_index,priority:2" json:"step_index"`
	StepKey              string                 `gorm:"type:varchar(128);not null;default:''" json:"step_key"`
	CapabilityID         string                 `gorm:"type:varchar(128);not null" json:"capability_id"`
	Title                string                 `gorm:"type:varchar(255);not null;default:''" json:"title"`
	Status               string                 `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	RiskLevel            string                 `gorm:"type:varchar(32);not null;default:'low'" json:"risk_level"`
	RequiresConfirmation bool                   `gorm:"not null;default:false" json:"requires_confirmation"`
	StartedAt            *time.Time             `json:"started_at,omitempty"`
	CompletedAt          *time.Time             `json:"completed_at,omitempty"`
	Error                *string                `gorm:"type:text" json:"error,omitempty"`
	Input                map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"input,omitempty"`
	Output               map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"output,omitempty"`
	Metadata             map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"metadata,omitempty"`
	CreatedAt            time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt            time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ActionStep) TableName() string {
	return "action_runtime_steps"
}
