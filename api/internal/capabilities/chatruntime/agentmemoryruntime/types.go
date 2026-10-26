package agentmemoryruntime

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

const (
	ToolMutate = "mutate_user_relationship_memory"

	legacyToolUpdate = "update_agent_memory"
	legacyToolClear  = "clear_agent_memory"
)

var ErrInvalidInput = errors.New("invalid input")

var zeroUUID = uuid.Nil

type Slot struct {
	Key         string `json:"key"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description"`
	MaxChars    int    `json:"max_chars"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
	// ReadOnly blocks are shown to the model but never written by it.
	ReadOnly bool `json:"read_only,omitempty"`
	// ManualOnly blocks take explicit user requests only.
	ManualOnly bool `json:"manual_only,omitempty"`
}

type State struct {
	Enabled                      bool
	AutoCaptureEnabled           bool
	RelationshipSettingsResolved bool
	AgentID                      uuid.UUID
	UserScope                    string
	MemoryEpoch                  *int64
	ConfigScope                  string
	ConfigRevision               string
	EnabledSlots                 []Slot
	SavedValues                  []agentmemory.SlotValueResponse
	ContextStatus                string
	ContextError                 string
}

type ToolOperation struct {
	Action   string `json:"action"`
	Key      string `json:"key"`
	Content  string `json:"content,omitempty"`
	Evidence string `json:"evidence"`
	Mode     string `json:"mode"`
}

type ToolArguments struct {
	Operations []ToolOperation `json:"operations"`
}

type MemoryService interface {
	ReadRuntimeFence(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, configScope, configRevision string) (int64, error)
	ReadUserMemory(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID) ([]agentmemory.SlotValueResponse, error)
	MutateValues(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID, req agentmemory.MutateValuesRequest, meta agentmemory.MutationMetadata) (*agentmemory.MutateValuesResponse, error)
}

// RelationshipSettingsService is optional so the legacy Agent memory service
// remains a valid rollback implementation during the compatibility window.
type RelationshipSettingsService interface {
	RelationshipRuntimeSettings(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (memoryEnabled bool, autoCaptureEnabled bool, err error)
}

type ContextRequest struct {
	SystemPrompt   string
	Enabled        bool
	Slots          []Slot
	MemoryService  MemoryService
	WorkspaceID    uuid.UUID
	AgentID        uuid.UUID
	UserID         uuid.UUID
	UserScope      string
	ConfigScope    string
	ConfigRevision string
	Budget         int
	// Blocks renders memory as files (ADR 0013) instead of the legacy list.
	Blocks bool
}

type ContextResult struct {
	SystemPrompt string
	Context      string
	Metadata     map[string]interface{}
	State        *State
}

type MutationRequest struct {
	MemoryService     MemoryService
	WorkspaceID       uuid.UUID
	AgentID           uuid.UUID
	UserID            uuid.UUID
	UserScope         string
	Slots             []Slot
	CurrentValues     []agentmemory.SlotValueResponse
	MutationMetadata  agentmemory.MutationMetadata
	LatestUserMessage string
	ProactiveAllowed  bool
	// RecentUserMessages are earlier user messages in the conversation that
	// memory_edit evidence may quote; CallID keeps one call's operations apart
	// from another's in the same turn.
	RecentUserMessages []string
	CallID             string
	// Archive keeps text moved out of a full file in the user's archive
	// (ADR 0014); nil when the memory store has no archive.
	Archive func(ctx context.Context, content, sourceKey string) error
}

type MutationResult struct {
	Status    string
	Arguments map[string]interface{}
	Result    map[string]interface{}
	Response  *agentmemory.MutateValuesResponse
	Error     error
}

func RuntimeSlots(input []Slot) []agentmemory.RuntimeSlot {
	out := make([]agentmemory.RuntimeSlot, 0, len(input))
	for _, slot := range input {
		if !slot.Enabled {
			continue
		}
		out = append(out, agentmemory.RuntimeSlot{
			Key:         slot.Key,
			Name:        slot.Name,
			Description: slot.Description,
			MaxChars:    slot.MaxChars,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
			ReadOnly:    slot.ReadOnly,
			ManualOnly:  slot.ManualOnly,
		})
	}
	return out
}
