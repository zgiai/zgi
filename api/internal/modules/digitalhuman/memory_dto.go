package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
)

type MemorySourceContext struct {
	PrivateConversation       bool `json:"private_conversation"`
	PromotedByAuthorizedActor bool `json:"promoted_by_authorized_actor"`
	// SourceTrust is set only by the runtime for audit; clients cannot supply it.
	SourceTrust SourceTrust `json:"-"`
}

type MemoryProposalRequest struct {
	OperationID                uuid.UUID                      `json:"operation_id"`
	Key                        string                         `json:"key"`
	Action                     memoryruntime.MutationAction   `json:"action"`
	MemoryType                 memoryruntime.MemoryType       `json:"memory_type"`
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
	// noteInstanceID routes the write to the DigitalHuman's notes on the end
	// user of this instance (ADR 0015); clients cannot set it.
	noteInstanceID string
	// internalOnly is set for writes below operator trust (ADR 0015): they
	// create internal memories and may change only internal ones.
	internalOnly    bool
	origin          MemoryOrigin
	originChannelID string
	// noteSourceKind records whether a note write was explicit or automatic
	// (ADR 0015).
	noteSourceKind string
}

type MemoryListQuery struct {
	Page     int
	PageSize int
	Type     memoryruntime.MemoryType
	Status   MemoryStatus
	Source   string
	Search   string
	// Visibility, Origin, and OriginChannelID filter the DigitalHuman's own
	// memory (ADR 0015).
	Visibility      MemoryVisibility
	Origin          MemoryOrigin
	OriginChannelID string
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

const (
	// MemoryProjectionRepairStatusRepaired reports a projection that was dropped
	// and rebuilt from the authoritative memory rows.
	MemoryProjectionRepairStatusRepaired = "repaired"
)

// MemoryProjectionRepairResult describes a completed projection rebuild.
type MemoryProjectionRepairResult struct {
	Status            string `json:"status"`
	MemoryCount       int    `json:"memory_count"`
	EmbeddingProvider string `json:"embedding_provider,omitempty"`
	EmbeddingModel    string `json:"embedding_model,omitempty"`
}
