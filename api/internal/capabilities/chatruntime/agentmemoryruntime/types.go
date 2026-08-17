package agentmemoryruntime

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

const (
	ToolMutate = "mutate_agent_memory"

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
}

type State struct {
	Enabled        bool
	AgentID        uuid.UUID
	UserScope      string
	MemoryEpoch    *int64
	ConfigScope    string
	ConfigRevision string
	EnabledSlots   []Slot
	SavedValues    []agentmemory.SlotValueResponse
	ContextStatus  string
	ContextError   string
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
		})
	}
	return out
}
