package usermemoryruntime

import (
	"context"
	"errors"

	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	ActionNone            = "none"
	ActionCreate          = "create"
	ActionUpdate          = "update"
	ActionDelete          = "delete"
	ActionAskConfirmation = "ask_confirmation"

	ToolCreate = "create_user_memory"
	ToolUpdate = "update_user_memory"
	ToolDelete = "delete_user_memory"
)

var ErrInvalidInput = errors.New("invalid user memory input")

type MemoryService interface {
	IsEnabled(ctx context.Context, accountID uuid.UUID) (bool, error)
	GetModelState(ctx context.Context, accountID uuid.UUID) (*memory.MemoryMeResponse, error)
	CreateEntryWithMetadata(ctx context.Context, accountID uuid.UUID, req memory.CreateEntryRequest, meta memory.MutationMetadata) (*memory.MemoryEntryResponse, error)
	UpdateEntryWithMetadata(ctx context.Context, accountID, entryID uuid.UUID, req memory.UpdateEntryRequest, meta memory.MutationMetadata) (*memory.MemoryEntryResponse, error)
	DeleteEntryWithMetadata(ctx context.Context, accountID, entryID uuid.UUID, meta memory.MutationMetadata) error
}

type State struct {
	AccountID uuid.UUID
	Entries   []memory.MemoryEntryResponse
}

type Decision struct {
	Action     string   `json:"action"`
	EntryID    string   `json:"entry_id,omitempty"`
	Content    string   `json:"content,omitempty"`
	Category   string   `json:"category,omitempty"`
	MemoryType string   `json:"memory_type,omitempty"`
	ExpiresAt  string   `json:"expires_at,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

type PreflightRequest struct {
	LatestUserMessage string
	LLMRequest        *adapter.ChatRequest
	State             *State
	MemoryService     MemoryService
	AccountID         uuid.UUID
	MutationMetadata  memory.MutationMetadata
	LLMClient         llmclient.LLMClient
	AppContext        *llmclient.AppContext
	UseJSONMode       bool
	OnMutation        func(trace skills.SkillTrace, result map[string]interface{})
}

type PreflightResult struct {
	Usage           *adapter.Usage
	Messages        []adapter.Message
	Traces          []skills.SkillTrace
	MetadataUpdates map[string]interface{}
}
