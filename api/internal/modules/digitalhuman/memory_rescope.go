package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// MemoryRescopeRequest moves one active memory to another memory class or
// business scope. Memories written before source trust existed are all self
// memory; operators use this to move shared business facts out of the
// DigitalHuman's identity, or to promote a business fact into it.
type MemoryRescopeRequest struct {
	OperationID      uuid.UUID                     `json:"operation_id"`
	ExpectedRevision int64                         `json:"expected_revision"`
	ConfigRevision   string                        `json:"config_revision"`
	MemoryEpoch      int64                         `json:"memory_epoch"`
	MemoryClass      memoryruntime.MemoryClass     `json:"memory_class"`
	Scope            *memoryruntime.MemoryScopeRef `json:"scope,omitempty"`
}

// RescopeMemory replaces a memory with the same content in the target class
// and scope. The original is invalidated in the same operation, which keeps
// the memory event history, records the origin in the action ledger, and can
// be undone like any other proposal.
//
// Only self memory and workspace business memory are valid targets. Channel
// and instance scopes are resolved by the runtime from verified bindings and
// are not chosen by hand.
func (s *Service) RescopeMemory(ctx context.Context, scope Scope, digitalHumanID, memoryID uuid.UUID, request MemoryRescopeRequest) (*MemoryMutationResult, error) {
	if request.OperationID == uuid.Nil || memoryID == uuid.Nil || request.ExpectedRevision < 1 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.memory.rescope")
	}
	target, err := rescopeTarget(scope, request)
	if err != nil {
		return nil, applicationError(err, "digital_human.memory.rescope")
	}
	var current *DigitalHumanMemory
	var replayed *MemoryMutationResult
	if err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		// A retried move has already invalidated its source, so a committed
		// operation is replayed before the source state is checked.
		if existing, eventErr := tx.MemoryEventByOperation(ctx, scope, digitalHumanID, request.OperationID); eventErr == nil {
			if existing.Result != "committed" || existing.MemoryID == nil {
				return ErrMemoryConflict
			}
			memory, memoryErr := tx.MemoryByIDForUpdate(ctx, scope, digitalHumanID, *existing.MemoryID)
			if memoryErr != nil {
				return memoryErr
			}
			replayed = &MemoryMutationResult{Memory: memory, Event: existing, Replayed: true}
			return nil
		} else if !recordNotFound(eventErr) {
			return eventErr
		}
		memory, findErr := tx.MemoryByIDForUpdate(ctx, scope, digitalHumanID, memoryID)
		current = memory
		return findErr
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrMemoryNotFound
		}
		return nil, applicationError(err, "digital_human.memory.rescope")
	}
	if replayed != nil {
		return replayed, nil
	}
	if current.Status != MemoryStatusActive || current.Revision != request.ExpectedRevision {
		return nil, applicationError(ErrMemoryConflict, "digital_human.memory.rescope")
	}
	if current.MemoryType == memoryruntime.MemoryTypeIdentity && target.class != memoryruntime.MemoryClassSelf {
		// Identity defines the DigitalHuman itself and never becomes business memory.
		return nil, applicationError(ErrMemoryPolicyRejected, "digital_human.memory.rescope")
	}
	if current.MemoryClass == target.class && current.ScopeType == target.scopeType() && current.ScopeID == target.scopeID() {
		return nil, applicationError(ErrInvalidRequest, "digital_human.memory.rescope")
	}
	var evidence []memoryruntime.MemoryEvidence
	_ = json.Unmarshal(current.Evidence, &evidence)
	evidence = append(evidence, memoryruntime.MemoryEvidence{Type: memoryruntime.EvidenceTypeManual, ID: "operator-rescope:" + request.OperationID.String()})
	result, err := s.applyMemoryProposal(ctx, scope, digitalHumanID, MemoryProposalRequest{
		OperationID: request.OperationID, Key: current.MemoryKey, Action: memoryruntime.MutationActionUpsert,
		MemoryClass: target.class, MemoryType: current.MemoryType, Scope: target.scope, Content: current.Content,
		SupersedesID: &current.ID, ExpectedSupersededRevision: current.Revision,
		ConfigRevision: request.ConfigRevision, MemoryEpoch: request.MemoryEpoch, Evidence: evidence,
		Visibility: current.Visibility, Confidence: current.Confidence,
		SourceContext: MemorySourceContext{PromotedByAuthorizedActor: true}, AdminApproved: true,
		OccurredAt: s.now(), rescope: true,
	}, false)
	if err != nil || result == nil || result.Replayed || current.ScopeType != memoryruntime.MemoryScopeInstance {
		return result, err
	}
	// Moving an entry out of an end user's instance is an audited promotion.
	if instanceID, parseErr := uuid.Parse(current.ScopeID); parseErr == nil {
		actorID := scope.ActorID
		instance := EndUserInstance{ID: instanceID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID}
		details := map[string]any{"memory_id": current.ID.String(), "to_class": string(target.class), "to_scope_type": string(target.scopeType())}
		if result.Memory != nil {
			details["promoted_memory_id"] = result.Memory.ID.String()
		}
		if auditErr := recordInstanceEvent(ctx, s.repo.db, instance, instanceAuditMemoryPromoted, accessAuditSourceConsole, &actorID, details); auditErr != nil {
			logger.WarnContext(ctx, "end-user instance memory promotion audit failed", "digital_human_id", digitalHumanID.String(), auditErr)
		}
	}
	return result, nil
}

type memoryRescopeTarget struct {
	class memoryruntime.MemoryClass
	scope *memoryruntime.MemoryScopeRef
}

func (t memoryRescopeTarget) scopeType() memoryruntime.MemoryScopeType {
	if t.scope == nil {
		return ""
	}
	return t.scope.Type
}

func (t memoryRescopeTarget) scopeID() string {
	if t.scope == nil {
		return ""
	}
	return t.scope.ID
}

func rescopeTarget(scope Scope, request MemoryRescopeRequest) (memoryRescopeTarget, error) {
	switch request.MemoryClass {
	case memoryruntime.MemoryClassSelf:
		if request.Scope != nil {
			return memoryRescopeTarget{}, ErrInvalidRequest
		}
		return memoryRescopeTarget{class: memoryruntime.MemoryClassSelf}, nil
	case memoryruntime.MemoryClassBusiness:
		if request.Scope == nil || request.Scope.Type != memoryruntime.MemoryScopeWorkspace {
			return memoryRescopeTarget{}, ErrInvalidRequest
		}
		// The workspace scope is always the caller's own workspace.
		return memoryRescopeTarget{class: memoryruntime.MemoryClassBusiness, scope: &memoryruntime.MemoryScopeRef{
			Type: memoryruntime.MemoryScopeWorkspace, ID: scope.WorkspaceID.String(),
		}}, nil
	default:
		return memoryRescopeTarget{}, ErrInvalidRequest
	}
}
