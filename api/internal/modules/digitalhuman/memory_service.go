package digitalhuman

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	maxMemoryContentBytes         = 4_000
	memoryUndoTTL                 = 24 * time.Hour
	memoryExtractionScheduleDelay = 10 * time.Second
	memoryExtractorVersion        = "digital-human-memory-v1"
)

var sensitiveMemoryPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|refresh[_-]?token|password|cookie|authorization)\s*[:=]`)

// ListMemories returns operator-visible memory and its current execution fence.
func (s *Service) ListMemories(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query MemoryListQuery) (*MemoryListResult, error) {
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.memory.list")
	}
	query = normalizeMemoryListQuery(query)
	if _, err := s.repo.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, "digital_human.memory.list")
	}
	items, total, err := s.repo.ListMemories(ctx, scope, digitalHumanID, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.memory.list")
	}
	for _, item := range items {
		item.Path = memoryFilePath(item)
	}
	state, err := s.memoryState(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, applicationError(err, "digital_human.memory.list")
	}
	return &MemoryListResult{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, HasMore: query.Page*query.PageSize < int(total), State: state}, nil
}

// UpdateMemoryState toggles the automatic extraction fuse and advances the
// subject epoch so every in-flight proposal and extraction job becomes stale.
func (s *Service) UpdateMemoryState(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request UpdateMemoryStateRequest) (*MemoryState, error) {
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil || request.ExpectedMemoryEpoch < 0 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.memory.state.update")
	}
	var updated *MemoryState
	err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		definition, err := tx.LockDefinition(ctx, scope, digitalHumanID)
		if err != nil {
			return err
		}
		published, err := tx.Published(ctx, scope, definition)
		if err != nil {
			return err
		}
		if published == nil {
			return ErrPublishInvalid
		}
		state, err := ensureMemoryState(ctx, tx, scope, digitalHumanID, published, s.now())
		if err != nil {
			return err
		}
		if state.MemoryEpoch != request.ExpectedMemoryEpoch {
			return ErrMemoryConflict
		}
		if state.AutoExtractionEnabled == request.AutoExtractionEnabled {
			updated = state
			return nil
		}
		previousEpoch, now := state.MemoryEpoch, s.now()
		state.MemoryEpoch++
		state.AutoExtractionEnabled, state.UpdatedAt = request.AutoExtractionEnabled, now
		if !request.AutoExtractionEnabled {
			state.ExtractionCutoffAt = &now
		}
		if err := tx.UpdateMemoryStateFence(ctx, state, previousEpoch); err != nil {
			return err
		}
		if err := tx.CancelMemoryExtractionJobs(ctx, scope, digitalHumanID, previousEpoch, now); err != nil {
			return err
		}
		detail := mustJSON(map[string]any{"auto_extraction_enabled": state.AutoExtractionEnabled, "memory_epoch": state.MemoryEpoch})
		if err := tx.CreateActionLedger(ctx, &ActionLedgerEntry{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
			ActionType: "memory.fuse", Status: "completed", Detail: detail, CreatedAt: now,
		}); err != nil {
			return err
		}
		updated = state
		return nil
	})
	if err != nil {
		return nil, applicationError(err, "digital_human.memory.state.update")
	}
	return updated, nil
}

// RepairMemoryProjection rebuilds the hidden vector projection for one
// DigitalHuman. Memory rows are the source of truth, so the rebuild only
// recreates derived data and never changes stored memory.
func (s *Service) RepairMemoryProjection(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) (*MemoryProjectionRepairResult, error) {
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.memory.repair")
	}
	if s.projectionRepairer == nil {
		// A nil repairer is a wiring gap: log it so operators can see the cause
		// behind the unavailable response.
		logger.WarnContext(ctx, "digital human memory projection repairer is not wired", "digital_human_id", digitalHumanID.String())
		return nil, applicationError(ErrMemoryProjectionUnavailable, "digital_human.memory.repair")
	}
	if _, err := s.repo.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, "digital_human.memory.repair")
	}
	result, err := s.projectionRepairer.Repair(ctx, memoryProjectionScope{
		OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
		ActorID: &scope.ActorID, PrincipalID: &scope.PrincipalID,
	})
	if err != nil {
		return nil, applicationError(fmt.Errorf("%w: %v", ErrMemoryProjectionRepairFailed, err), "digital_human.memory.repair")
	}
	if err := s.repo.CreateActionLedger(ctx, &ActionLedgerEntry{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
		ActionType: "memory.projection.repair", Status: "completed", CreatedAt: s.now(),
		Detail: mustJSON(map[string]any{"memory_count": result.MemoryCount, "embedding_provider": result.EmbeddingProvider, "embedding_model": result.EmbeddingModel}),
	}); err != nil {
		return nil, applicationError(err, "digital_human.memory.repair")
	}
	return result, nil
}

// ScheduleMemoryExtraction records a reference-only extraction job after a
// successful runtime turn. Content remains in the run/message stores and is
// loaded transiently by the worker.
func (s *Service) ScheduleMemoryExtraction(
	ctx context.Context,
	scope Scope,
	run *Run,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	artifactRefs datatypes.JSON,
) error {
	if !validScopeBoundary(scope) || run == nil || run.ID == uuid.Nil || run.DigitalHumanID == uuid.Nil ||
		run.OrganizationID != scope.OrganizationID || run.WorkspaceID != scope.WorkspaceID ||
		conversationID == uuid.Nil || messageID == uuid.Nil || !json.Valid(artifactRefs) {
		return applicationError(ErrInvalidRequest, "digital_human.memory.extraction.schedule")
	}
	var refs []map[string]any
	if err := json.Unmarshal(artifactRefs, &refs); err != nil || len(refs) > 100 {
		return applicationError(ErrInvalidRequest, "digital_human.memory.extraction.schedule")
	}
	now := s.now()
	return s.repo.Transaction(ctx, func(tx *GormRepository) error {
		state, err := tx.MemoryStateForUpdate(ctx, scope, run.DigitalHumanID)
		if err != nil {
			return err
		}
		if !state.AutoExtractionEnabled {
			return nil
		}
		if !backgroundExtractionAdmitted(run) {
			return nil
		}
		var version Version
		if err := tx.db.WithContext(ctx).Where(
			"id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND state = ?",
			run.DigitalHumanVersionID, scope.OrganizationID, scope.WorkspaceID, run.DigitalHumanID, VersionStatePublished,
		).Take(&version).Error; err != nil {
			return err
		}
		if version.Revision != state.ConfigRevision {
			return nil
		}
		runID, conversationRef, messageRef := run.ID, conversationID, messageID
		job := &MemoryExtractionJob{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: run.DigitalHumanID, RunID: &runID, ConversationID: &conversationRef, MessageID: &messageRef,
			ArtifactRefs: append(datatypes.JSON(nil), artifactRefs...), IdempotencyKey: "run:" + run.ID.String(),
			ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch, ExtractorVersion: memoryExtractorVersion,
			Status: MemoryExtractionJobPending, ScheduledAt: now.Add(memoryExtractionScheduleDelay), CreatedAt: now, UpdatedAt: now,
		}
		created, err := tx.CreateMemoryExtractionJob(ctx, job)
		if err != nil || !created {
			return err
		}
		detail := mustJSON(map[string]any{
			"job_id": job.ID, "run_id": run.ID, "conversation_id": conversationID, "message_id": messageID,
			"artifact_refs": refs, "config_revision": state.ConfigRevision, "memory_epoch": state.MemoryEpoch,
		})
		return tx.CreateActionLedger(ctx, &ActionLedgerEntry{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: run.DigitalHumanID, RunID: &runID, ActionType: "memory.extraction", Status: "proposed",
			Detail: detail, CreatedAt: now,
		})
	})
}

// ApplyMemoryProposal applies an administrator-authored proposal. Autonomous
// callers must use ApplyAutonomousMemoryProposal so identity policy cannot be bypassed.
func (s *Service) ApplyMemoryProposal(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request MemoryProposalRequest) (*MemoryMutationResult, error) {
	return s.applyMemoryProposal(ctx, scope, digitalHumanID, request, false)
}

// ApplyAutonomousMemoryProposal applies the narrower model/tool-authored policy.
func (s *Service) ApplyAutonomousMemoryProposal(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request MemoryProposalRequest) (*MemoryMutationResult, error) {
	return s.applyMemoryProposal(ctx, scope, digitalHumanID, request, true)
}

func (s *Service) applyMemoryProposal(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request MemoryProposalRequest, autonomous bool) (*MemoryMutationResult, error) {
	if err := validateMemorySupersedeContract(request); err != nil {
		s.recordRejectedMemoryProposal(ctx, scope, digitalHumanID, request, err)
		return nil, applicationError(err, "digital_human.memory.propose")
	}
	mutation := memoryruntime.MemoryMutation{
		OperationID: request.OperationID,
		Subject:     memoryruntime.MemorySubjectRef{DigitalHumanID: digitalHumanID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID},
		Key:         request.Key, Action: request.Action, Class: request.MemoryClass, Type: request.MemoryType, Scope: request.Scope,
		Content: request.Content, ExpectedRevision: request.ExpectedRevision,
		Fence:    memoryruntime.MemoryFence{ConfigRevision: request.ConfigRevision, MemoryEpoch: request.MemoryEpoch},
		Evidence: request.Evidence, SourcePrincipalID: scope.ActorID.String(), OccurredAt: request.OccurredAt,
	}
	validated, err := memoryruntime.ValidateMemoryMutation(mutation)
	if err != nil {
		cause := ErrInvalidRequest
		if request.Action == memoryruntime.MutationActionUpsert && len(request.Evidence) == 0 {
			cause = ErrMemoryPolicyRejected
			s.recordRejectedMemoryProposal(ctx, scope, digitalHumanID, request, cause)
		}
		return nil, applicationError(cause, "digital_human.memory.propose")
	}
	request.Key, request.Content, request.Scope, request.Evidence = validated.Key, validated.Content, validated.Scope, validated.Evidence
	request.ConfigRevision, request.MemoryEpoch, request.OccurredAt = validated.Fence.ConfigRevision, validated.Fence.MemoryEpoch, validated.OccurredAt
	if request.origin == "" {
		request.origin = MemoryOriginOperator
	}
	if request.Confidence == 0 {
		request.Confidence = 1
	}
	if err := validateMemoryProposalPolicy(request, autonomous); err != nil {
		s.recordRejectedMemoryProposal(ctx, scope, digitalHumanID, request, err)
		return nil, applicationError(err, "digital_human.memory.propose")
	}
	// The DigitalHuman's notes on an end user live in that person's
	// relationship memory (ADR 0015).
	if request.Scope != nil && request.Scope.Type == memoryruntime.MemoryScopeInstance {
		result, err := s.applyInstanceNote(ctx, scope, digitalHumanID, request)
		if err != nil {
			if errors.Is(err, errInstanceMemoryFull) || errors.Is(err, ErrMemoryPolicyRejected) {
				s.recordRejectedMemoryProposal(ctx, scope, digitalHumanID, request, err)
			}
			return nil, applicationError(err, "digital_human.memory.propose")
		}
		return result, nil
	}

	var result *MemoryMutationResult
	err = s.repo.Transaction(ctx, func(tx *GormRepository) error {
		if existing, eventErr := tx.MemoryEventByOperation(ctx, scope, digitalHumanID, request.OperationID); eventErr == nil {
			if existing.Result != "committed" || existing.MemoryID == nil {
				return ErrMemoryPolicyRejected
			}
			memory, memoryErr := tx.MemoryByIDForUpdate(ctx, scope, digitalHumanID, *existing.MemoryID)
			if memoryErr != nil {
				return memoryErr
			}
			result = &MemoryMutationResult{Memory: memory, Event: existing, Replayed: true}
			return nil
		} else if !recordNotFound(eventErr) {
			return eventErr
		}
		definition, lockErr := tx.LockDefinition(ctx, scope, digitalHumanID)
		if lockErr != nil {
			return lockErr
		}
		published, publishedErr := tx.Published(ctx, scope, definition)
		if publishedErr != nil {
			return publishedErr
		}
		if published == nil {
			return ErrPublishInvalid
		}
		state, stateErr := ensureMemoryState(ctx, tx, scope, definition.ID, published, s.now())
		if stateErr != nil {
			return stateErr
		}
		if request.ConfigRevision != state.ConfigRevision || request.MemoryEpoch != state.MemoryEpoch {
			return ErrMemoryConflict
		}
		if request.RunID != nil {
			if _, runErr := tx.RunByID(ctx, scope, digitalHumanID, *request.RunID); runErr != nil {
				return ErrMemoryPolicyRejected
			}
		}
		if capacityErr := checkInstanceMemoryCapacity(ctx, tx, scope, digitalHumanID, request); capacityErr != nil {
			return capacityErr
		}
		memory, before, supersededBefore, applyErr := applyMemoryMutation(ctx, tx, scope, digitalHumanID, request, state, s.now())
		if applyErr != nil {
			return applyErr
		}
		evidence, _ := json.Marshal(request.Evidence)
		after, _ := json.Marshal(memory)
		event := &MemoryEvent{
			ID: uuid.New(), OperationID: request.OperationID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: digitalHumanID, MemoryID: &memory.ID, Action: string(request.Action), Result: "committed",
			BeforeSnapshot: before, AfterSnapshot: after, Evidence: evidence, SourcePrincipalID: scope.ActorID.String(), RunID: request.RunID, CreatedAt: s.now(),
		}
		if len(before) != 0 {
			value := request.ExpectedRevision
			event.BeforeRevision = &value
		}
		afterRevision := memory.Revision
		event.AfterRevision = &afterRevision
		if err := tx.CreateMemoryEvent(ctx, event); err != nil {
			return err
		}
		undo := &MemoryUndoRecord{
			OperationID: request.OperationID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: digitalHumanID, MemoryID: memory.ID, PreviousSnapshot: before, SupersededSnapshot: supersededBefore,
			ResultingRevision: memory.Revision, ExpiresAt: s.now().Add(memoryUndoTTL), CreatedAt: s.now(),
		}
		if err := tx.CreateMemoryUndo(ctx, undo); err != nil {
			return err
		}
		ledger := map[string]any{
			"memory_class": memory.MemoryClass, "memory_type": memory.MemoryType,
			"scope_type": memory.ScopeType, "scope_id": memory.ScopeID, "supersedes_id": memory.SupersedesID,
		}
		if request.rescope {
			ledger["rescoped_from"] = request.SupersedesID
		}
		detail, _ := json.Marshal(ledger)
		if err := tx.CreateActionLedger(ctx, &ActionLedgerEntry{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
			RunID: request.RunID, ActionType: "memory." + string(request.Action), Status: "committed",
			MemoryOperationID: &request.OperationID, MemoryID: &memory.ID, Detail: detail, CreatedAt: s.now(),
		}); err != nil {
			return err
		}
		result = &MemoryMutationResult{Memory: memory, Event: event}
		return nil
	})
	if errors.Is(err, errInstanceMemoryFull) {
		s.recordRejectedMemoryProposal(ctx, scope, digitalHumanID, request, err)
	}
	if err != nil {
		return nil, applicationError(err, "digital_human.memory.propose")
	}
	return result, nil
}

func applyMemoryMutation(ctx context.Context, tx *GormRepository, scope Scope, digitalHumanID uuid.UUID, request MemoryProposalRequest, state *MemoryState, now time.Time) (*DigitalHumanMemory, datatypes.JSON, datatypes.JSON, error) {
	scopeType, scopeID := memoryruntime.MemoryScopeType(""), ""
	if request.Scope != nil {
		scopeType, scopeID = request.Scope.Type, request.Scope.ID
	}
	current, err := tx.MemoryByKeyForUpdate(ctx, scope, digitalHumanID, request.MemoryClass, scopeType, scopeID, request.Key)
	if recordNotFound(err) {
		if request.ExpectedRevision != 0 || request.Action == memoryruntime.MutationActionClear {
			return nil, nil, nil, ErrMemoryConflict
		}
		supersededBefore, err := invalidateSupersededMemory(ctx, tx, scope, digitalHumanID, request, scopeType, scopeID, state, now)
		if err != nil {
			return nil, nil, nil, err
		}
		validFrom := request.OccurredAt
		visibility := request.Visibility
		if visibility == "" {
			visibility = MemoryVisibilityEveryone
		}
		supersedesID := request.SupersedesID
		if request.rescope {
			// A supersede link stays within one scope. A rescoped memory is
			// linked to its origin by the undo record and the action ledger.
			supersedesID = nil
		}
		memory := &DigitalHumanMemory{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
			MemoryKey: request.Key, MemoryClass: request.MemoryClass, MemoryType: request.MemoryType, ScopeType: scopeType, ScopeID: scopeID,
			Content: request.Content, ContentDigest: memoryContentDigest(request.Content), Visibility: visibility,
			Origin: request.origin, OriginChannelID: request.originChannelID,
			Status: MemoryStatusActive, Confidence: request.Confidence, Revision: 1, ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
			Evidence: mustJSON(request.Evidence), SourcePrincipalID: scope.ActorID.String(), LastOperationID: request.OperationID,
			SupersedesID: supersedesID, ValidFrom: &validFrom, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.CreateMemory(ctx, memory); err != nil {
			return nil, nil, nil, err
		}
		return memory, nil, supersededBefore, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	var supersededBefore datatypes.JSON
	if request.rescope {
		// A rescope may land on a key the target scope used before, for
		// example after a previous move was undone. An active memory there
		// is a real conflict; an invalidated one is reactivated in place.
		if current.Status == MemoryStatusActive {
			return nil, nil, nil, ErrMemoryConflict
		}
		request.ExpectedRevision = current.Revision
		if supersededBefore, err = invalidateSupersededMemory(ctx, tx, scope, digitalHumanID, request, scopeType, scopeID, state, now); err != nil {
			return nil, nil, nil, err
		}
	}
	if current.Revision != request.ExpectedRevision {
		return nil, nil, nil, ErrMemoryConflict
	}
	if request.internalOnly && current.Visibility != MemoryVisibilityInternal {
		return nil, nil, nil, errMemoryEveryoneOperatorOnly
	}
	before, _ := json.Marshal(current)
	current.Revision++
	current.LastOperationID, current.SourcePrincipalID, current.ConfigRevision, current.MemoryEpoch = request.OperationID, scope.ActorID.String(), state.ConfigRevision, state.MemoryEpoch
	current.Evidence, current.UpdatedAt = mustJSON(request.Evidence), now
	if request.Action == memoryruntime.MutationActionClear {
		current.Status, current.ValidTo = MemoryStatusInvalidated, &now
	} else {
		current.MemoryType, current.Content, current.ContentDigest = request.MemoryType, request.Content, memoryContentDigest(request.Content)
		if request.Visibility != "" {
			current.Visibility = request.Visibility
		}
		current.Confidence, current.Status, current.ValidTo = request.Confidence, MemoryStatusActive, nil
		validFrom := request.OccurredAt
		current.ValidFrom = &validFrom
	}
	if err := tx.UpdateMemoryCAS(ctx, current, request.ExpectedRevision); err != nil {
		return nil, nil, nil, err
	}
	return current, before, supersededBefore, nil
}

// invalidateSupersededMemory invalidates the memory a proposal supersedes and
// returns its snapshot for undo. A normal supersede stays within one scope; a
// rescope always crosses scopes.
func invalidateSupersededMemory(ctx context.Context, tx *GormRepository, scope Scope, digitalHumanID uuid.UUID, request MemoryProposalRequest, scopeType memoryruntime.MemoryScopeType, scopeID string, state *MemoryState, now time.Time) (datatypes.JSON, error) {
	if request.SupersedesID == nil {
		return nil, nil
	}
	superseded, err := tx.MemoryByIDForUpdate(ctx, scope, digitalHumanID, *request.SupersedesID)
	if recordNotFound(err) {
		return nil, ErrMemoryConflict
	}
	if err != nil {
		return nil, err
	}
	if request.internalOnly && superseded.Visibility != MemoryVisibilityInternal {
		return nil, errMemoryEveryoneOperatorOnly
	}
	sameScope := superseded.MemoryClass == request.MemoryClass && superseded.ScopeType == scopeType && superseded.ScopeID == scopeID
	if superseded.Status != MemoryStatusActive || superseded.Revision != request.ExpectedSupersededRevision || sameScope == request.rescope {
		return nil, ErrMemoryConflict
	}
	snapshot := mustJSON(superseded)
	expectedRevision := superseded.Revision
	superseded.Revision++
	superseded.Status, superseded.ValidTo = MemoryStatusInvalidated, &now
	superseded.ConfigRevision, superseded.MemoryEpoch = state.ConfigRevision, state.MemoryEpoch
	superseded.SourcePrincipalID, superseded.LastOperationID = scope.ActorID.String(), request.OperationID
	superseded.UpdatedAt = now
	if err := tx.UpdateMemoryCAS(ctx, superseded, expectedRevision); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// UndoMemory restores the exact previous snapshot if the target change is still current.
func (s *Service) UndoMemory(ctx context.Context, scope Scope, digitalHumanID, operationID uuid.UUID, request MemoryUndoRequest) (*MemoryMutationResult, error) {
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil || operationID == uuid.Nil || request.ExpectedRevision < 1 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.memory.undo")
	}
	var result *MemoryMutationResult
	err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		undo, err := tx.MemoryUndoForUpdate(ctx, scope, digitalHumanID, operationID)
		if recordNotFound(err) {
			return ErrMemoryUndoUnavailable
		}
		if err != nil {
			return err
		}
		now := s.now()
		if undo.ConsumedAt != nil || !now.Before(undo.ExpiresAt) || undo.ResultingRevision != request.ExpectedRevision {
			return ErrMemoryUndoUnavailable
		}
		memory, err := tx.MemoryByIDForUpdate(ctx, scope, digitalHumanID, undo.MemoryID)
		if recordNotFound(err) {
			return ErrMemoryNotFound
		}
		if err != nil {
			return err
		}
		if memory.Revision != request.ExpectedRevision || memory.LastOperationID != operationID {
			return ErrMemoryConflict
		}
		before, _ := json.Marshal(memory)
		if len(undo.PreviousSnapshot) == 0 {
			memory.Status, memory.ValidTo = MemoryStatusInvalidated, &now
		} else {
			var previous DigitalHumanMemory
			if err := json.Unmarshal(undo.PreviousSnapshot, &previous); err != nil {
				return fmt.Errorf("decode memory undo snapshot: %w", err)
			}
			restoreMemorySnapshot(memory, &previous)
		}
		undoOperationID := uuid.New()
		memory.Revision, memory.LastOperationID, memory.UpdatedAt = request.ExpectedRevision+1, undoOperationID, now
		if err := tx.UpdateMemoryCAS(ctx, memory, request.ExpectedRevision); err != nil {
			return err
		}
		resultMemory := memory
		var restoredMemory *DigitalHumanMemory
		if len(undo.SupersededSnapshot) != 0 {
			var previous DigitalHumanMemory
			if err := json.Unmarshal(undo.SupersededSnapshot, &previous); err != nil {
				return fmt.Errorf("decode superseded memory undo snapshot: %w", err)
			}
			restoredMemory, err = tx.MemoryByIDForUpdate(ctx, scope, digitalHumanID, previous.ID)
			if recordNotFound(err) {
				return ErrMemoryUndoUnavailable
			}
			if err != nil {
				return err
			}
			expectedSupersededRevision := previous.Revision + 1
			if restoredMemory.Revision != expectedSupersededRevision || restoredMemory.LastOperationID != operationID || restoredMemory.Status != MemoryStatusInvalidated {
				return ErrMemoryConflict
			}
			restoreMemorySnapshot(restoredMemory, &previous)
			restoredMemory.Revision, restoredMemory.LastOperationID, restoredMemory.UpdatedAt = expectedSupersededRevision+1, undoOperationID, now
			if err := tx.UpdateMemoryCAS(ctx, restoredMemory, expectedSupersededRevision); err != nil {
				return err
			}
			resultMemory = restoredMemory
		}
		if err := tx.ConsumeMemoryUndo(ctx, operationID, now); err != nil {
			return err
		}
		after, _ := json.Marshal(memory)
		event := &MemoryEvent{
			ID: uuid.New(), OperationID: undoOperationID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			DigitalHumanID: digitalHumanID, MemoryID: &memory.ID, Action: "undo", Result: "committed",
			BeforeSnapshot: before, AfterSnapshot: after, SourcePrincipalID: scope.ActorID.String(), Evidence: datatypes.JSON([]byte(`[]`)), CreatedAt: now,
		}
		beforeRevision, afterRevision := request.ExpectedRevision, memory.Revision
		event.BeforeRevision, event.AfterRevision = &beforeRevision, &afterRevision
		if restoredMemory != nil {
			event.ReasonCode = "supersede_reverted"
		}
		if err := tx.CreateMemoryEvent(ctx, event); err != nil {
			return err
		}
		detail := map[string]any{"reverted_memory_id": memory.ID, "reverted_revision": memory.Revision}
		if restoredMemory != nil {
			detail["restored_memory_id"], detail["restored_revision"] = restoredMemory.ID, restoredMemory.Revision
		}
		if err := tx.CreateActionLedger(ctx, &ActionLedgerEntry{
			ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
			ActionType: "memory.undo", Status: "reverted", MemoryOperationID: &event.OperationID, MemoryID: &memory.ID,
			Detail: mustJSON(detail), CreatedAt: now,
		}); err != nil {
			return err
		}
		result = &MemoryMutationResult{Memory: resultMemory, Event: event}
		return nil
	})
	if err != nil {
		return nil, applicationError(err, "digital_human.memory.undo")
	}
	return result, nil
}

// RetrieveMemories returns the DigitalHuman's active memories. Convergence: a
// DigitalHuman currently owns a single self memory store, so business scopes no
// longer filter recall; legacy business rows stay visible. Actor identity never
// participates in the storage key.
func (s *Service) RetrieveMemories(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, limit int) ([]*DigitalHumanMemory, error) {
	return s.retrieveMemories(ctx, scope, digitalHumanID, limit, nil)
}

// retrieveMemories optionally limits recall to one run's readable scope.
func (s *Service) retrieveMemories(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, limit int, readScope *memoryReadScope) ([]*DigitalHumanMemory, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	items, err := s.repo.ActiveMemories(ctx, scope, digitalHumanID, limit, readScope)
	if err != nil {
		return nil, applicationError(err, "digital_human.memory.retrieve")
	}
	return items, nil
}

func (s *Service) memoryState(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) (*MemoryState, error) {
	var state *MemoryState
	err := s.repo.Transaction(ctx, func(tx *GormRepository) error {
		definition, err := tx.LockDefinition(ctx, scope, digitalHumanID)
		if err != nil {
			return err
		}
		published, err := tx.Published(ctx, scope, definition)
		if err != nil {
			return err
		}
		if published == nil {
			return ErrPublishInvalid
		}
		state, err = ensureMemoryState(ctx, tx, scope, digitalHumanID, published, s.now())
		return err
	})
	return state, err
}

func ensureMemoryState(ctx context.Context, tx *GormRepository, scope Scope, digitalHumanID uuid.UUID, published *Version, now time.Time) (*MemoryState, error) {
	state, err := tx.MemoryStateForUpdate(ctx, scope, digitalHumanID)
	if recordNotFound(err) {
		state = &MemoryState{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID, ConfigRevision: published.Revision, MemoryEpoch: 0, AutoExtractionEnabled: false, CreatedAt: now, UpdatedAt: now}
		if err := tx.CreateMemoryState(ctx, state); err != nil {
			return nil, err
		}
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if state.ConfigRevision != published.Revision {
		previous := state.MemoryEpoch
		state.ConfigRevision, state.MemoryEpoch, state.UpdatedAt = published.Revision, previous+1, now
		if err := tx.UpdateMemoryStateFence(ctx, state, previous); err != nil {
			return nil, err
		}
		if err := tx.CancelMemoryExtractionJobs(ctx, scope, digitalHumanID, previous, now); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func validateMemoryProposalPolicy(request MemoryProposalRequest, autonomous bool) error {
	if len(request.Content) > maxMemoryContentBytes || request.Confidence < 0 || request.Confidence > 1 {
		return ErrMemoryPolicyRejected
	}
	if request.Visibility != "" && request.Visibility != MemoryVisibilityEveryone && request.Visibility != MemoryVisibilityInternal {
		return ErrMemoryPolicyRejected
	}
	if request.internalOnly && request.Visibility != MemoryVisibilityInternal {
		return errMemoryEveryoneOperatorOnly
	}
	// A private console conversation reaches memory everyone sees only
	// through an authorized operator; internal memory is shared inside the
	// team by design (ADR 0015).
	if request.SourceContext.PrivateConversation && !request.SourceContext.PromotedByAuthorizedActor && !request.internalOnly {
		return ErrMemoryPolicyRejected
	}
	if sensitiveMemoryPattern.MatchString(request.Content) || looksLikeStructuredBusinessObject(request.Content) {
		return ErrMemoryPolicyRejected
	}
	// Convergence: self is the only memory class, so an autonomous proposal may
	// carry any non-identity type, including evidence-backed facts. Partitioning
	// facts behind a business scope is deferred as a future extension.
	if autonomous && request.MemoryType == memoryruntime.MemoryTypeIdentity {
		return ErrMemoryPolicyRejected
	}
	if request.MemoryType == memoryruntime.MemoryTypeIdentity && !request.AdminApproved {
		return ErrMemoryPolicyRejected
	}
	return nil
}

func validateMemorySupersedeContract(request MemoryProposalRequest) error {
	if request.SupersedesID == nil {
		if request.ExpectedSupersededRevision != 0 {
			return ErrInvalidRequest
		}
		return nil
	}
	if *request.SupersedesID == uuid.Nil || request.Action != memoryruntime.MutationActionUpsert ||
		request.ExpectedRevision != 0 || request.ExpectedSupersededRevision < 1 {
		return ErrInvalidRequest
	}
	return nil
}

func restoreMemorySnapshot(memory, previous *DigitalHumanMemory) {
	memory.MemoryType, memory.Content, memory.ContentDigest = previous.MemoryType, previous.Content, previous.ContentDigest
	memory.Visibility, memory.Status, memory.Confidence = previous.Visibility, previous.Status, previous.Confidence
	memory.ConfigRevision, memory.MemoryEpoch = previous.ConfigRevision, previous.MemoryEpoch
	memory.Evidence, memory.SourcePrincipalID = previous.Evidence, previous.SourcePrincipalID
	memory.SupersedesID, memory.ValidFrom, memory.ValidTo = previous.SupersedesID, previous.ValidFrom, previous.ValidTo
}

func looksLikeStructuredBusinessObject(content string) bool {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return false
	}
	var value any
	return json.Unmarshal([]byte(trimmed), &value) == nil
}

func (s *Service) recordRejectedMemoryProposal(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request MemoryProposalRequest, cause error) {
	if request.OperationID == uuid.Nil || !validScopeBoundary(scope) || digitalHumanID == uuid.Nil {
		return
	}
	_ = s.repo.Transaction(ctx, func(tx *GormRepository) error {
		if _, err := tx.Definition(ctx, scope, digitalHumanID); err != nil {
			return err
		}
		if _, err := tx.MemoryEventByOperation(ctx, scope, digitalHumanID, request.OperationID); err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		evidence := mustJSON(request.Evidence)
		event := &MemoryEvent{ID: uuid.New(), OperationID: request.OperationID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID, Action: "reject", Result: "rejected", Evidence: evidence, SourcePrincipalID: scope.ActorID.String(), RunID: request.RunID, ReasonCode: memoryRejectionReason(cause), CreatedAt: s.now()}
		if err := tx.CreateMemoryEvent(ctx, event); err != nil {
			return err
		}
		values := map[string]any{"reason_code": event.ReasonCode, "memory_class": request.MemoryClass, "memory_type": request.MemoryType}
		if request.SourceContext.SourceTrust != "" {
			values["source_trust"] = request.SourceContext.SourceTrust
		}
		if request.Scope != nil {
			values["scope_type"] = request.Scope.Type
		}
		detail := mustJSON(values)
		return tx.CreateActionLedger(ctx, &ActionLedgerEntry{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID, RunID: request.RunID, ActionType: "memory.proposal", Status: "rejected", MemoryOperationID: &request.OperationID, Detail: detail, CreatedAt: s.now()})
	})
}

func memoryRejectionReason(cause error) string {
	switch {
	case errors.Is(cause, errMemorySourceTrustRejected):
		return memoryAdmissionReasonTrust
	case errors.Is(cause, errMemoryChannelScopeUnresolved):
		return memoryAdmissionReasonChannelScope
	case errors.Is(cause, ErrMemoryConflict):
		return "memory_fence_conflict"
	case errors.Is(cause, ErrInvalidRequest):
		return "memory_contract_invalid"
	default:
		return "memory_policy_rejected"
	}
}

func normalizeMemoryListQuery(query MemoryListQuery) MemoryListQuery {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = defaultPageSize
	}
	if query.PageSize > maxPageSize {
		query.PageSize = maxPageSize
	}
	query.Search, query.ScopeID, query.Source = strings.TrimSpace(query.Search), strings.TrimSpace(query.ScopeID), strings.TrimSpace(query.Source)
	return query
}

func memoryContentDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func mustJSON(value any) datatypes.JSON {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal validated DigitalHuman memory value: %v", err))
	}
	return encoded
}
