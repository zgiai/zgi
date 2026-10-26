package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/gorm"
)

// MemoryVisibilityRequest changes who may see one of the DigitalHuman's own
// memories (ADR 0015): everyone, or only runs inside the team.
type MemoryVisibilityRequest struct {
	OperationID      uuid.UUID        `json:"operation_id"`
	ExpectedRevision int64            `json:"expected_revision"`
	ConfigRevision   string           `json:"config_revision"`
	MemoryEpoch      int64            `json:"memory_epoch"`
	Visibility       MemoryVisibility `json:"visibility"`
}

// SetMemoryVisibility makes a memory visible to everyone or internal only. It
// is an operator action, recorded and undoable like any other change.
func (s *Service) SetMemoryVisibility(ctx context.Context, scope Scope, digitalHumanID, memoryID uuid.UUID, request MemoryVisibilityRequest) (*MemoryMutationResult, error) {
	const operation = "digital_human.memory.visibility"
	if request.OperationID == uuid.Nil || memoryID == uuid.Nil || request.ExpectedRevision < 1 ||
		(request.Visibility != MemoryVisibilityEveryone && request.Visibility != MemoryVisibilityInternal) {
		return nil, applicationError(ErrInvalidRequest, operation)
	}
	var current *DigitalHumanMemory
	err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		memory, findErr := tx.MemoryByIDForUpdate(ctx, scope, digitalHumanID, memoryID)
		current = memory
		return findErr
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrMemoryNotFound
		}
		return nil, applicationError(err, operation)
	}
	if current.MemoryClass != memoryruntime.MemoryClassSelf || current.Status != MemoryStatusActive || current.Revision != request.ExpectedRevision {
		return nil, applicationError(ErrMemoryConflict, operation)
	}
	var evidence []memoryruntime.MemoryEvidence
	_ = json.Unmarshal(current.Evidence, &evidence)
	evidence = append(evidence, memoryruntime.MemoryEvidence{Type: memoryruntime.EvidenceTypeManual, ID: "operator-visibility:" + request.OperationID.String()})
	return s.applyMemoryProposal(ctx, scope, digitalHumanID, MemoryProposalRequest{
		OperationID: request.OperationID, Key: current.MemoryKey, Action: memoryruntime.MutationActionUpsert,
		MemoryClass: memoryruntime.MemoryClassSelf, MemoryType: current.MemoryType, Content: current.Content,
		ExpectedRevision: current.Revision, ConfigRevision: request.ConfigRevision, MemoryEpoch: request.MemoryEpoch,
		Evidence: evidence, Visibility: request.Visibility, Confidence: current.Confidence,
		SourceContext: MemorySourceContext{PromotedByAuthorizedActor: true}, AdminApproved: true,
		OccurredAt: s.now(),
	}, false)
}
