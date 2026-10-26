package digitalhuman

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/gorm"
)

// maxMemoryBulkItems bounds one bulk request, matching the list page size.
const maxMemoryBulkItems = maxPageSize

// MemoryBulkAction is what a bulk request does to each selected memory.
type MemoryBulkAction string

const (
	// MemoryBulkSetVisibility makes each memory visible to everyone or
	// internal only.
	MemoryBulkSetVisibility MemoryBulkAction = "set_visibility"
	// MemoryBulkInvalidate invalidates each memory, for example everything
	// one channel taught the DigitalHuman.
	MemoryBulkInvalidate MemoryBulkAction = "invalidate"
)

// MemoryBulkRequest applies one operator action to several of the
// DigitalHuman's memories (ADR 0015). Each memory changes in its own recorded,
// undoable mutation, whose operation ID derives from OperationID so a retried
// request replays instead of applying twice.
type MemoryBulkRequest struct {
	OperationID uuid.UUID        `json:"operation_id"`
	Action      MemoryBulkAction `json:"action"`
	Visibility  MemoryVisibility `json:"visibility,omitempty"`
	MemoryIDs   []uuid.UUID      `json:"memory_ids"`
}

// MemoryBulkSkip names a memory the bulk action left unchanged and why.
type MemoryBulkSkip struct {
	MemoryID uuid.UUID `json:"memory_id"`
	Reason   string    `json:"reason"`
}

// MemoryBulkResult reports how many memories changed and which were skipped.
type MemoryBulkResult struct {
	Updated int              `json:"updated"`
	Skipped []MemoryBulkSkip `json:"skipped"`
}

const (
	memoryBulkSkipNotFound  = "not_found"
	memoryBulkSkipInactive  = "inactive"
	memoryBulkSkipUnchanged = "unchanged"
	memoryBulkSkipIdentity  = "identity"
	memoryBulkSkipConflict  = "conflict"
)

// ApplyMemoryBulk applies an operator action to each selected memory in turn.
// A memory that cannot change is skipped with a reason; the rest still apply.
func (s *Service) ApplyMemoryBulk(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request MemoryBulkRequest) (*MemoryBulkResult, error) {
	const operation = "digital_human.memory.bulk"
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil || request.OperationID == uuid.Nil ||
		len(request.MemoryIDs) == 0 || len(request.MemoryIDs) > maxMemoryBulkItems {
		return nil, applicationError(ErrInvalidRequest, operation)
	}
	switch request.Action {
	case MemoryBulkInvalidate:
	case MemoryBulkSetVisibility:
		if request.Visibility != MemoryVisibilityEveryone && request.Visibility != MemoryVisibilityInternal {
			return nil, applicationError(ErrInvalidRequest, operation)
		}
	default:
		return nil, applicationError(ErrInvalidRequest, operation)
	}
	state, err := s.memoryState(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	result := &MemoryBulkResult{Skipped: []MemoryBulkSkip{}}
	seen := make(map[uuid.UUID]bool, len(request.MemoryIDs))
	for _, memoryID := range request.MemoryIDs {
		if memoryID == uuid.Nil || seen[memoryID] {
			continue
		}
		seen[memoryID] = true
		reason, err := s.applyMemoryBulkItem(ctx, scope, digitalHumanID, state, request, memoryID)
		if err != nil {
			return nil, applicationError(err, operation)
		}
		if reason != "" {
			result.Skipped = append(result.Skipped, MemoryBulkSkip{MemoryID: memoryID, Reason: reason})
			continue
		}
		result.Updated++
	}
	return result, nil
}

// applyMemoryBulkItem changes one memory, returning a skip reason when it
// cannot or need not change.
func (s *Service) applyMemoryBulkItem(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, state *MemoryState, request MemoryBulkRequest, memoryID uuid.UUID) (string, error) {
	current, err := s.repo.MemoryByID(ctx, scope, digitalHumanID, memoryID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return memoryBulkSkipNotFound, nil
	}
	if err != nil {
		return "", err
	}
	if current.Status != MemoryStatusActive {
		return memoryBulkSkipInactive, nil
	}
	operationID := uuid.NewSHA1(request.OperationID, memoryID[:])
	switch request.Action {
	case MemoryBulkSetVisibility:
		if current.MemoryType == memoryruntime.MemoryTypeIdentity {
			return memoryBulkSkipIdentity, nil
		}
		if current.Visibility == request.Visibility {
			return memoryBulkSkipUnchanged, nil
		}
		_, err = s.SetMemoryVisibility(ctx, scope, digitalHumanID, memoryID, MemoryVisibilityRequest{
			OperationID: operationID, ExpectedRevision: current.Revision,
			ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch, Visibility: request.Visibility,
		})
	case MemoryBulkInvalidate:
		_, err = s.applyMemoryProposal(ctx, scope, digitalHumanID, MemoryProposalRequest{
			OperationID: operationID, Key: current.MemoryKey, Action: memoryruntime.MutationActionClear,
			MemoryType: current.MemoryType, ExpectedRevision: current.Revision,
			ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
			Evidence:      []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeManual, ID: "operator-bulk:" + request.OperationID.String()}},
			SourceContext: MemorySourceContext{PromotedByAuthorizedActor: true}, AdminApproved: true,
			OccurredAt: s.now(),
		}, false)
	}
	if errors.Is(err, ErrMemoryConflict) {
		return memoryBulkSkipConflict, nil
	}
	return "", err
}
