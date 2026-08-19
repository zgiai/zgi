package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
)

type MemorySourceContext struct {
	PrivateConversation       bool `json:"private_conversation"`
	PromotedByAuthorizedActor bool `json:"promoted_by_authorized_actor"`
}

type MemoryProposalRequest struct {
	OperationID                uuid.UUID                      `json:"operation_id"`
	Key                        string                         `json:"key"`
	Action                     memoryruntime.MutationAction   `json:"action"`
	MemoryClass                memoryruntime.MemoryClass      `json:"memory_class"`
	MemoryType                 memoryruntime.MemoryType       `json:"memory_type"`
	Scope                      *memoryruntime.MemoryScopeRef  `json:"scope,omitempty"`
	Content                    string                         `json:"content,omitempty"`
	ExpectedRevision           int64                          `json:"expected_revision"`
	SupersedesID               *uuid.UUID                     `json:"supersedes_id,omitempty"`
	ExpectedSupersededRevision int64                          `json:"expected_superseded_revision,omitempty"`
	ConfigRevision             string                         `json:"config_revision"`
	MemoryEpoch                int64                          `json:"memory_epoch"`
	Evidence                   []memoryruntime.MemoryEvidence `json:"evidence,omitempty"`
	Visibility                 MemoryVisibility               `json:"visibility,omitempty"`
	Confidence                 float64                        `json:"confidence,omitempty"`
	RunID                      *uuid.UUID                     `json:"run_id,omitempty"`
	SourceContext              MemorySourceContext            `json:"source_context"`
	AdminApproved              bool                           `json:"admin_approved"`
	OccurredAt                 time.Time                      `json:"occurred_at"`
}

type MemoryListQuery struct {
	Page      int
	PageSize  int
	Class     memoryruntime.MemoryClass
	Type      memoryruntime.MemoryType
	ScopeType memoryruntime.MemoryScopeType
	ScopeID   string
	Status    MemoryStatus
	Source    string
	Search    string
}

type MemoryListResult struct {
	Items    []*DigitalHumanMemory `json:"items"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	HasMore  bool                  `json:"has_more"`
	State    *MemoryState          `json:"state"`
}

type MemoryMutationResult struct {
	Memory   *DigitalHumanMemory `json:"memory,omitempty"`
	Event    *MemoryEvent        `json:"event"`
	Replayed bool                `json:"replayed"`
}

type MemoryUndoRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type UpdateMemoryStateRequest struct {
	ExpectedMemoryEpoch   int64 `json:"expected_memory_epoch"`
	AutoExtractionEnabled bool  `json:"auto_extraction_enabled"`
}
