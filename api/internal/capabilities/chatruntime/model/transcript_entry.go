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
)

// TranscriptFormatVersionV1 is the protocol-message format written by the first
// release of the transcript entry table. Readers must refuse unknown versions
// instead of guessing.
const TranscriptFormatVersionV1 = 1

// TranscriptEntry is one protocol message (the LLM-level message) of one turn.
//
// It is append-only:
//   - Payload is the original protocol message and never changes.
//   - Preview is the effective form the model sees. It may be rewritten by
//     budget pruning (large tool results become "ref + excerpt"), but it must
//     always be recomputable from Payload plus the pruning policy.
//
// CSeq is monotonic per conversation and is the address space for context
// summaries and projection slices; Seq is the position inside one message.
type TranscriptEntry struct {
	ID             uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	MessageID      uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_chat_runtime_transcript_entries_message_seq,priority:1" json:"message_id"`
	ConversationID uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_chat_runtime_transcript_entries_cseq,priority:1" json:"conversation_id"`
	OrganizationID uuid.UUID              `gorm:"type:uuid;not null" json:"organization_id"`
	CSeq           int64                  `gorm:"column:cseq;not null;uniqueIndex:uq_chat_runtime_transcript_entries_cseq,priority:2" json:"cseq"`
	Seq            int                    `gorm:"not null;uniqueIndex:uq_chat_runtime_transcript_entries_message_seq,priority:2" json:"seq"`
	Kind           string                 `gorm:"type:varchar(32);not null" json:"kind"`
	Preview        map[string]interface{} `gorm:"type:jsonb;serializer:json;not null" json:"preview"`
	Payload        map[string]interface{} `gorm:"type:jsonb;serializer:json;not null" json:"payload"`
	ByteSize       int                    `gorm:"not null;default:0" json:"byte_size"`
	CreatedAt      time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (TranscriptEntry) TableName() string {
	return "chat_runtime_message_transcript_entries"
}

// TranscriptEntryPreview is the payload-free view of a transcript entry.
// Context projection and every list path must use this type so that a query
// never detoasts the large payload column.
type TranscriptEntryPreview struct {
	ID        uuid.UUID              `json:"id"`
	MessageID uuid.UUID              `json:"message_id"`
	CSeq      int64                  `gorm:"column:cseq" json:"cseq"`
	Seq       int                    `json:"seq"`
	Kind      string                 `json:"kind"`
	Preview   map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"preview"`
	ByteSize  int                    `json:"byte_size"`
}

// MessageContextSummary is the newest context summary applicable at or before a
// boundary position.
type MessageContextSummary struct {
	MessageID  uuid.UUID `json:"message_id"`
	Summary    string    `json:"summary"`
	ThroughSeq int64     `json:"through_seq"`
}
