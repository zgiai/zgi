package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	RuntimeInputStatusPending  = "pending"
	RuntimeInputStatusClaimed  = "claimed"
	RuntimeInputStatusConsumed = "consumed"
	RuntimeInputStatusCanceled = "canceled"
	RuntimeInputStatusFailed   = "failed"

	RuntimeInputConsumptionCurrentRun = "current_run"
	RuntimeInputConsumptionFollowUp   = "follow_up"
)

// RuntimeInput is a durable user message submitted while a ChatRuntime turn is
// active. It belongs to one conversation and can only be consumed by the
// active turn or by the deterministic follow-up turn created for that turn.
type RuntimeInput struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID    uuid.UUID  `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID       *uuid.UUID `gorm:"type:uuid" json:"workspace_id,omitempty"`
	AccountID         uuid.UUID  `gorm:"type:uuid;not null" json:"account_id"`
	ConversationID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_chat_runtime_inputs_conversation_sequence,priority:1" json:"conversation_id"`
	TargetMessageID   uuid.UUID  `gorm:"type:uuid;not null;index:idx_chat_runtime_inputs_target_status_sequence,priority:1" json:"target_message_id"`
	ConsumedMessageID *uuid.UUID `gorm:"type:uuid" json:"consumed_message_id,omitempty"`
	ClientInputID     uuid.UUID  `gorm:"type:uuid;not null" json:"client_input_id"`
	Sequence          int64      `gorm:"not null;index:idx_chat_runtime_inputs_conversation_sequence,priority:2;index:idx_chat_runtime_inputs_target_status_sequence,priority:3" json:"sequence"`
	Content           string     `gorm:"type:text;not null" json:"content"`
	Status            string     `gorm:"type:varchar(32);not null;index:idx_chat_runtime_inputs_target_status_sequence,priority:2" json:"status"`
	ConsumptionMode   *string    `gorm:"type:varchar(32)" json:"consumption_mode,omitempty"`
	ClaimBatchID      *uuid.UUID `gorm:"type:uuid;index:idx_chat_runtime_inputs_claim_batch" json:"claim_batch_id,omitempty"`
	ClaimedAt         *time.Time `json:"claimed_at,omitempty"`
	ClaimExpiresAt    *time.Time `json:"claim_expires_at,omitempty"`
	ConsumedAt        *time.Time `json:"consumed_at,omitempty"`
	CanceledAt        *time.Time `json:"canceled_at,omitempty"`
	Error             *string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt         time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (RuntimeInput) TableName() string { return "chat_runtime_inputs" }
