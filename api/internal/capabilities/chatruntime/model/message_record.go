package model

import (
	"time"

	"github.com/google/uuid"
)

// Message record kinds. Each is a list that used to live in message metadata
// under the same key (ADR 0039).
const (
	MessageRecordKindModelInvocation = "model_invocations"
)

// MessageRecord is one entry of a per-message list that grows while a turn
// runs, such as one model call. Records are written one at a time instead of
// rewriting the message's metadata, and are read back into metadata only by
// the paths that show them (ADR 0039).
type MessageRecord struct {
	ID             uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	MessageID      uuid.UUID              `gorm:"type:uuid;not null" json:"message_id"`
	OrganizationID uuid.UUID              `gorm:"type:uuid;not null" json:"organization_id"`
	Kind           string                 `gorm:"type:varchar(64);not null" json:"kind"`
	RecordID       string                 `gorm:"type:varchar(255);not null" json:"record_id"`
	Sequence       int                    `gorm:"not null;default:0" json:"sequence"`
	Payload        map[string]interface{} `gorm:"type:jsonb;serializer:json;not null" json:"payload"`
	CreatedAt      time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (MessageRecord) TableName() string {
	return "chat_runtime_message_records"
}

// SystemPrompt keeps one copy of a system prompt that model call records
// refer to by hash, instead of a copy in every record.
type SystemPrompt struct {
	Hash      string    `gorm:"type:varchar(64);primaryKey" json:"hash"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (SystemPrompt) TableName() string {
	return "chat_runtime_system_prompts"
}
