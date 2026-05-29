package agentmemoryruntime

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	ToolUpdate = "update_agent_memory"
	ToolClear  = "clear_agent_memory"

	maxPlanningRounds  = 3
	maxPlanningRetries = 1

	minDecisionConfidence = 0.55
)

var ErrInvalidInput = errors.New("invalid input")

var zeroUUID = uuid.Nil

type Slot struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	MaxChars    int    `json:"max_chars"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}

type State struct {
	Enabled       bool
	AgentID       uuid.UUID
	UserScope     string
	EnabledSlots  []Slot
	SavedValues   []agentmemory.SlotValueResponse
	ContextStatus string
	ContextError  string
}

type Decision struct {
	Action     string   `json:"action"`
	Key        string   `json:"key,omitempty"`
	Content    string   `json:"content,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

type PlannerResult struct {
	Status   string
	Decision Decision
	Error    error
}

type MutationResult struct {
	Status string
	Key    string
	Result map[string]interface{}
	Error  error
}

type MemoryService interface {
	ReadUserMemory(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID) ([]agentmemory.SlotValueResponse, error)
	UpdateValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID, req agentmemory.UpdateValueRequest, meta agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error)
	ClearValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID, key string, meta agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error)
}

type ContextRequest struct {
	SystemPrompt  string
	Enabled       bool
	Slots         []Slot
	MemoryService MemoryService
	WorkspaceID   uuid.UUID
	AgentID       uuid.UUID
	UserID        uuid.UUID
	UserScope     string
	Budget        int
}

type ContextResult struct {
	SystemPrompt string
	Metadata     map[string]interface{}
	State        *State
}

type PreflightRequest struct {
	LatestUserMessage string
	LLMRequest        *adapter.ChatRequest
	State             *State
	MemoryService     MemoryService
	WorkspaceID       uuid.UUID
	AgentID           uuid.UUID
	UserID            uuid.UUID
	UserScope         string
	MutationMetadata  agentmemory.MutationMetadata
	LLMClient         llmclient.LLMClient
	AppContext        *llmclient.AppContext
	UseJSONMode       bool
	OnToolCallStart   func(toolName string, arguments map[string]interface{})
	OnToolCallEnd     func(trace skills.SkillTrace)
}

type PreflightResult struct {
	Usage           *adapter.Usage
	Messages        []adapter.Message
	Traces          []skills.SkillTrace
	MetadataUpdates map[string]interface{}
}

type MutationRequest struct {
	MemoryService    MemoryService
	WorkspaceID      uuid.UUID
	AgentID          uuid.UUID
	UserID           uuid.UUID
	UserScope        string
	Slots            []Slot
	MutationMetadata agentmemory.MutationMetadata
	OnToolCallStart  func(toolName string, arguments map[string]interface{})
	OnToolCallEnd    func(trace skills.SkillTrace)
}

func RuntimeSlots(input []Slot) []agentmemory.RuntimeSlot {
	out := make([]agentmemory.RuntimeSlot, 0, len(input))
	for _, slot := range input {
		if !slot.Enabled {
			continue
		}
		out = append(out, agentmemory.RuntimeSlot{
			Key:         slot.Key,
			Description: slot.Description,
			MaxChars:    slot.MaxChars,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
		})
	}
	return out
}
