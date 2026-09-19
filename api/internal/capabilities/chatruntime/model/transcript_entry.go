package model

import (
	"time"

	"github.com/google/uuid"
)

// Protocol message roles stored as transcript entries.
const (
	TranscriptEntryKindUser      = "user"
	TranscriptEntryKindAssistant = "assistant"
	TranscriptEntryKindTool      = "tool"
	TranscriptEntryKindSummary   = "summary"
)

// TranscriptEntry is one protocol message (the LLM-level message) of one turn.
//
// It is append-only:
//   - Payload is the original protocol message and never changes.
//   - Preview is the effective form the model sees. It may be rewritten by
//     budget pruning (large tool results become "ref + excerpt"), but it must
//     always be recomputable from Payload plus the pruning policy.
type TranscriptEntry struct {
	ID                   uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	MessageID            uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_chat_runtime_transcript_entries_message_event,priority:1,where:event_id <> ''" json:"message_id"`
	ConversationID       uuid.UUID              `gorm:"type:uuid;not null;index" json:"conversation_id"`
	OrganizationID       uuid.UUID              `gorm:"type:uuid;not null" json:"organization_id"`
	ParentEntryID        *uuid.UUID             `gorm:"type:uuid;index" json:"-"`
	CoversThroughEntryID *uuid.UUID             `gorm:"type:uuid;index" json:"-"`
	EventID              string                 `gorm:"type:varchar(255);not null;default:'';uniqueIndex:uq_chat_runtime_transcript_entries_message_event,priority:2,where:event_id <> ''" json:"-"`
	Kind                 string                 `gorm:"type:varchar(32);not null" json:"kind"`
	Preview              map[string]interface{} `gorm:"type:jsonb;serializer:json;not null" json:"preview"`
	Payload              map[string]interface{} `gorm:"type:jsonb;serializer:json;not null" json:"payload"`
	ByteSize             int                    `gorm:"not null;default:0" json:"byte_size"`
	CreatedAt            time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (TranscriptEntry) TableName() string {
	return "chat_runtime_message_transcript_entries"
}

// TranscriptEntryPreview is the payload-free view of a transcript entry.
// Context projection and every list path must use this type so that a query
// never detoasts the large payload column.
type TranscriptEntryPreview struct {
	ID                   uuid.UUID              `json:"id"`
	MessageID            uuid.UUID              `json:"message_id"`
	ParentEntryID        *uuid.UUID             `json:"-"`
	CoversThroughEntryID *uuid.UUID             `json:"-"`
	Kind                 string                 `json:"kind"`
	Preview              map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"preview"`
	ByteSize             int                    `json:"byte_size"`
}
