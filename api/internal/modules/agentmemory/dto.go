package agentmemory

import (
	"time"

	"github.com/google/uuid"
)

type SlotResponse struct {
	ID               string `json:"id"`
	Key              string `json:"key"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	MaxChars         int    `json:"max_chars"`
	Enabled          bool   `json:"enabled"`
	SortOrder        int    `json:"sort_order"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
	CreatedAtUnix    int64  `json:"created_at_unix"`
	UpdatedAtUnix    int64  `json:"updated_at_unix"`
	CreatedAtISO     string `json:"created_at_iso"`
	UpdatedAtISO     string `json:"updated_at_iso"`
	CreatedAtDisplay string `json:"created_at_display"`
	UpdatedAtDisplay string `json:"updated_at_display"`
}

type SlotValueResponse struct {
	SlotResponse
	Content              string `json:"content"`
	Revision             int64  `json:"revision"`
	SourceKind           string `json:"source_kind"`
	SourceConversationID string `json:"source_conversation_id,omitempty"`
	SourceMessageID      string `json:"source_message_id,omitempty"`
	SourceCompletedAt    int64  `json:"source_completed_at,omitempty"`
	ExtractorVersion     string `json:"extractor_version,omitempty"`
	LastOperationID      string `json:"last_operation_id,omitempty"`
	UndoableUntil        *int64 `json:"undoable_until,omitempty"`
}

type ReplaceSlotsRequest struct {
	Slots []SlotUpsertRequest `json:"slots" binding:"required"`
}

type SlotUpsertRequest struct {
	ID          string `json:"id,omitempty"`
	Key         string `json:"key" binding:"required"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MaxChars    int    `json:"max_chars,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
	SortOrder   int    `json:"sort_order,omitempty"`
}

type UpdateValueRequest struct {
	Key              string `json:"key" binding:"required"`
	Content          string `json:"content" binding:"required"`
	ExpectedRevision *int64 `json:"expected_revision,omitempty"`
}

const (
	MutationActionUpsert = "upsert"
	MutationActionClear  = "clear"

	MutationModeExplicit  = "explicit"
	MutationModeProactive = "proactive"

	MutationStatusUpdated   = "updated"
	MutationStatusCleared   = "cleared"
	MutationStatusUnchanged = "unchanged"
)

// ValueMutation is an internal, revision-bound mutation. SourceMessageID and
// SourceCompletedAt may override the batch metadata when a background batch
// contains evidence from more than one completed turn.
type ValueMutation struct {
	Action            string
	Key               string
	Content           string
	Mode              string
	ExpectedRevision  int64
	OperationID       uuid.UUID
	SourceMessageID   *uuid.UUID
	SourceCompletedAt *time.Time
}

type MutateValuesRequest struct {
	Operations []ValueMutation
}

type ValueMutationResult struct {
	Action        string `json:"action"`
	Status        string `json:"status"`
	Key           string `json:"key"`
	Revision      int64  `json:"revision"`
	SourceKind    string `json:"source_kind"`
	OperationID   string `json:"operation_id"`
	UndoableUntil *int64 `json:"undoable_until,omitempty"`
}

type MutateValuesResponse struct {
	Status     string                `json:"status"`
	Operations []ValueMutationResult `json:"operations"`
}

type MemoryExportResponse struct {
	AgentID    string              `json:"agent_id"`
	UserScope  string              `json:"user_scope"`
	UserID     string              `json:"user_id"`
	ExportedAt int64               `json:"exported_at"`
	Values     []SlotValueResponse `json:"values"`
}

type UndoResponse struct {
	OperationID string             `json:"operation_id"`
	Value       *SlotValueResponse `json:"value,omitempty"`
}

type ScheduleExtractionRequest struct {
	WorkspaceID        string
	AgentID            string
	UserScope          string
	UserID             string
	ConversationID     string
	MessageWatermarkID string
	ExtractorVersion   string
	ConfigScope        string
	ConfigRevision     string
	Slots              []RuntimeSlot
}
