package agentmemory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const inlineAgentMemoryExtractorVersion = "inline-agent-memory-v1"

type normalizedValueMutation struct {
	ValueMutation
	slot       RuntimeSlot
	sourceKind string
}

// MutateValues validates and applies one revision-bound batch atomically. It
// serializes batches per subject so absent-row creates and clears participate
// in the same consistency boundary as ordinary CAS updates.
func (s *Service) MutateValues(
	ctx context.Context,
	workspaceID, agentID uuid.UUID,
	slots []RuntimeSlot,
	userScope string,
	userID uuid.UUID,
	req MutateValuesRequest,
	meta MutationMetadata,
) (*MutateValuesResponse, error) {
	if s == nil || s.repo == nil || workspaceID == uuid.Nil || agentID == uuid.Nil || userID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	userScope, err := s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return nil, err
	}
	operations, err := normalizeValueMutations(req.Operations, slots)
	if err != nil {
		return nil, err
	}
	meta = normalizeMutationMetadata(meta)
	requiresEpoch := false
	for _, operation := range operations {
		if operation.sourceKind == SourceKindAutomatic {
			requiresEpoch = true
			break
		}
	}
	if requiresEpoch && meta.MemoryEpoch == nil {
		return nil, fmt.Errorf("%w: automatic memory epoch is required", ErrInvalidInput)
	}
	if !requiresEpoch {
		if response, complete, receiptErr := mutationReceiptResponse(ctx, s.repo, workspaceID, agentID, userScope, userID, operations); receiptErr != nil {
			return nil, receiptErr
		} else if complete {
			return response, nil
		}
	}
	var response *MutateValuesResponse
	err = s.repo.WithTransaction(ctx, func(tx store) error {
		state, lockErr := tx.LockSubjectState(ctx, workspaceID, agentID, userScope, userID)
		if lockErr != nil {
			return lockErr
		}
		if meta.MemoryEpoch != nil && state.MemoryEpoch != *meta.MemoryEpoch {
			return ErrConflict
		}
		if receipt, complete, receiptErr := mutationReceiptResponse(ctx, tx, workspaceID, agentID, userScope, userID, operations); receiptErr != nil {
			return receiptErr
		} else if complete {
			response = receipt
			return nil
		}

		beforeByKey := make(map[string]*AgentMemoryValue, len(operations))
		legacyReplay := make(map[string]bool, len(operations))
		for _, operation := range operations {
			before, getErr := tx.GetValueScopedForUpdate(ctx, workspaceID, agentID, operation.Key, userScope, userID)
			if getErr != nil && !errors.Is(getErr, gorm.ErrRecordNotFound) {
				return fmt.Errorf("get agent memory value: %w", getErr)
			}
			if before != nil && before.LastOperationID != nil && *before.LastOperationID == operation.OperationID {
				legacyReplay[operation.Key] = true
				beforeByKey[operation.Key] = before
				continue
			}
			currentRevision := int64(0)
			if before != nil {
				currentRevision = before.Revision
			}
			if operation.ExpectedRevision != currentRevision {
				return ErrConflict
			}
			sourceCompletedAt := mutationSourceCompletedAt(operation, meta)
			if before != nil && operation.sourceKind == SourceKindAutomatic && sourceCompletedAt != nil &&
				(before.SourceKind == SourceKindExplicit || before.SourceKind == SourceKindManager) && before.UpdatedAt.After(*sourceCompletedAt) {
				return ErrConflict
			}
			beforeByKey[operation.Key] = before
		}

		results := make([]ValueMutationResult, 0, len(operations))
		invalidate := false
		for _, operation := range operations {
			before := beforeByKey[operation.Key]
			opMeta := mutationOperationMetadata(meta, operation)
			if legacyReplay[operation.Key] {
				opMeta.EventResult = MutationStatusUnchanged
				if eventErr := recordValueEvent(ctx, tx, workspaceID, agentID, operation.Key, userScope, userID, EventActionValueUpdate, opMeta, before, before); eventErr != nil {
					return eventErr
				}
				results = append(results, mutationResult(operation, MutationStatusUnchanged, before, nil))
				continue
			}

			switch operation.Action {
			case MutationActionUpsert:
				if before != nil && before.Content == operation.Content {
					opMeta.EventResult = MutationStatusUnchanged
					if eventErr := recordValueEvent(ctx, tx, workspaceID, agentID, operation.Key, userScope, userID, EventActionValueUpdate, opMeta, before, before); eventErr != nil {
						return eventErr
					}
					results = append(results, mutationResult(operation, MutationStatusUnchanged, before, nil))
					continue
				}
				after, writeErr := writeBatchValue(ctx, tx, workspaceID, agentID, userScope, userID, operation, before, opMeta)
				if writeErr != nil {
					return writeErr
				}
				opMeta.EventResult = MutationStatusUpdated
				if eventErr := recordValueEvent(ctx, tx, workspaceID, agentID, operation.Key, userScope, userID, EventActionValueUpdate, opMeta, before, after); eventErr != nil {
					return eventErr
				}
				var undoableUntil *time.Time
				if operation.sourceKind == SourceKindAutomatic {
					record := undoRecordForAutomaticWrite(operation.OperationID, after, before)
					if undoErr := tx.CreateUndoRecord(ctx, record); undoErr != nil {
						return fmt.Errorf("create agent memory undo record: %w", undoErr)
					}
					undoableUntil = &record.ExpiresAt
				}
				results = append(results, mutationResult(operation, MutationStatusUpdated, after, undoableUntil))
			case MutationActionClear:
				invalidate = true
				status := MutationStatusUnchanged
				if before != nil {
					if clearErr := tx.DeleteValueCAS(ctx, before, before.Revision); clearErr != nil {
						return fmt.Errorf("clear agent memory value: %w", clearErr)
					}
					status = MutationStatusCleared
				}
				if undoErr := tx.DeleteUndoForSlot(ctx, workspaceID, agentID, userScope, userID, operation.Key); undoErr != nil {
					return fmt.Errorf("delete agent memory undo records: %w", undoErr)
				}
				opMeta.EventResult = status
				if eventErr := recordValueEvent(ctx, tx, workspaceID, agentID, operation.Key, userScope, userID, EventActionValueClear, opMeta, before, nil); eventErr != nil {
					return eventErr
				}
				results = append(results, mutationResult(operation, status, nil, nil))
			}
		}
		if invalidate {
			if cancelErr := tx.CancelPendingJobsForSubject(ctx, workspaceID, agentID, userScope, userID); cancelErr != nil {
				return cancelErr
			}
			cutoff := meta.SourceCompletedAt
			if cutoff == nil {
				now := time.Now()
				cutoff = &now
			}
			if state.ExtractionCutoffAt == nil || state.ExtractionCutoffAt.Before(*cutoff) {
				value := *cutoff
				state.ExtractionCutoffAt = &value
			}
			if updateErr := tx.UpdateSubjectEpoch(ctx, state, state.MemoryEpoch+1); updateErr != nil {
				return updateErr
			}
		}
		response = &MutateValuesResponse{Status: "success", Operations: results}
		return nil
	})
	if err == nil {
		return response, nil
	}
	if !requiresEpoch {
		if response, complete, receiptErr := mutationReceiptResponse(ctx, s.repo, workspaceID, agentID, userScope, userID, operations); receiptErr == nil && complete {
			return response, nil
		}
	}
	if isDuplicateKeyError(err) {
		return nil, ErrConflict
	}
	return nil, err
}

func normalizeValueMutations(input []ValueMutation, slots []RuntimeSlot) ([]normalizedValueMutation, error) {
	if len(input) == 0 || len(input) > 5 {
		return nil, fmt.Errorf("%w: memory batch must contain between one and five operations", ErrInvalidInput)
	}
	seenKeys := make(map[string]struct{}, len(input))
	seenOperations := make(map[uuid.UUID]struct{}, len(input))
	operations := make([]normalizedValueMutation, 0, len(input))
	for _, raw := range input {
		key, err := normalizeKey(raw.Key)
		if err != nil {
			return nil, err
		}
		if _, exists := seenKeys[key]; exists {
			return nil, fmt.Errorf("%w: duplicate memory key %s", ErrInvalidInput, key)
		}
		if raw.OperationID == uuid.Nil {
			return nil, fmt.Errorf("%w: operation id is required", ErrInvalidInput)
		}
		if _, exists := seenOperations[raw.OperationID]; exists {
			return nil, fmt.Errorf("%w: duplicate operation id", ErrInvalidInput)
		}
		slot, err := runtimeSlotByKey(slots, key)
		if err != nil {
			return nil, err
		}
		action := strings.ToLower(strings.TrimSpace(raw.Action))
		mode := strings.ToLower(strings.TrimSpace(raw.Mode))
		if mode != MutationModeExplicit && mode != MutationModeProactive {
			return nil, fmt.Errorf("%w: unsupported memory mutation mode", ErrInvalidInput)
		}
		if action != MutationActionUpsert && action != MutationActionClear {
			return nil, fmt.Errorf("%w: unsupported memory mutation action", ErrInvalidInput)
		}
		if mode == MutationModeProactive && action != MutationActionUpsert {
			return nil, fmt.Errorf("%w: proactive mutation is not allowed for slot %s", ErrUnauthorized, key)
		}
		content := strings.TrimSpace(raw.Content)
		if action == MutationActionUpsert {
			if content == "" {
				return nil, fmt.Errorf("%w: memory content is required", ErrInvalidInput)
			}
			if len([]rune(content)) > slot.MaxChars {
				return nil, fmt.Errorf("%w: content exceeds max_chars for %s", ErrInvalidInput, key)
			}
			if ContainsSensitiveContent(content) {
				return nil, fmt.Errorf("%w: sensitive content cannot be saved", ErrInvalidInput)
			}
		}
		sourceKind := SourceKindExplicit
		if mode == MutationModeProactive {
			sourceKind = SourceKindAutomatic
		}
		raw.Key, raw.Action, raw.Mode, raw.Content = key, action, mode, content
		operations = append(operations, normalizedValueMutation{ValueMutation: raw, slot: slot, sourceKind: sourceKind})
		seenKeys[key] = struct{}{}
		seenOperations[raw.OperationID] = struct{}{}
	}
	sort.SliceStable(operations, func(i, j int) bool { return operations[i].Key < operations[j].Key })
	return operations, nil
}

func writeBatchValue(ctx context.Context, tx store, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, operation normalizedValueMutation, before *AgentMemoryValue, meta MutationMetadata) (*AgentMemoryValue, error) {
	revision := int64(1)
	if before != nil {
		revision = before.Revision + 1
	}
	value := &AgentMemoryValue{
		ID:                   uuid.New(),
		WorkspaceID:          workspaceID,
		AgentID:              agentID,
		SlotKey:              operation.Key,
		UserScope:            userScope,
		UserID:               userID,
		Content:              operation.Content,
		Revision:             revision,
		SourceKind:           operation.sourceKind,
		SourceConversationID: meta.SourceConversationID,
		SourceMessageID:      meta.SourceMessageID,
		SourceCompletedAt:    meta.SourceCompletedAt,
		ExtractorVersion:     meta.ExtractorVersion,
		LastOperationID:      meta.OperationID,
	}
	if before == nil {
		if err := tx.CreateValue(ctx, value); err != nil {
			return nil, fmt.Errorf("create agent memory value: %w", err)
		}
		return value, nil
	}
	value.ID = before.ID
	value.CreatedAt = before.CreatedAt
	if err := tx.UpdateValueCAS(ctx, value, before.Revision); err != nil {
		return nil, fmt.Errorf("update agent memory value: %w", err)
	}
	return value, nil
}

func mutationOperationMetadata(base MutationMetadata, operation normalizedValueMutation) MutationMetadata {
	meta := base
	operationID := operation.OperationID
	meta.OperationID = &operationID
	meta.SourceKind = operation.sourceKind
	if operation.sourceKind == SourceKindAutomatic && strings.TrimSpace(meta.ExtractorVersion) == "" {
		meta.ExtractorVersion = inlineAgentMemoryExtractorVersion
	}
	if operation.SourceMessageID != nil {
		meta.SourceMessageID = operation.SourceMessageID
	}
	if operation.SourceCompletedAt != nil {
		meta.SourceCompletedAt = operation.SourceCompletedAt
	}
	return meta
}

func mutationSourceCompletedAt(operation normalizedValueMutation, base MutationMetadata) *time.Time {
	if operation.SourceCompletedAt != nil {
		return operation.SourceCompletedAt
	}
	return base.SourceCompletedAt
}

func mutationResult(operation normalizedValueMutation, status string, value *AgentMemoryValue, undoableUntil *time.Time) ValueMutationResult {
	result := ValueMutationResult{
		Action:      operation.Action,
		Status:      status,
		Key:         operation.Key,
		SourceKind:  operation.sourceKind,
		OperationID: operation.OperationID.String(),
	}
	if value != nil {
		result.Revision = value.Revision
	}
	if undoableUntil != nil {
		unix := undoableUntil.Unix()
		result.UndoableUntil = &unix
	}
	return result
}

func mutationReceiptResponse(ctx context.Context, repo store, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, operations []normalizedValueMutation) (*MutateValuesResponse, bool, error) {
	results := make([]ValueMutationResult, 0, len(operations))
	found := 0
	for _, operation := range operations {
		event, err := repo.GetEventByOperationID(ctx, operation.OperationID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		found++
		if event.WorkspaceID != workspaceID || event.AgentID != agentID || event.UserID == nil || *event.UserID != userID || event.UserScope != userScope || event.SlotKey != operation.Key {
			return nil, false, ErrConflict
		}
		status := strings.TrimSpace(event.Result)
		if status == "" || status == "success" {
			status = MutationStatusUpdated
			if operation.Action == MutationActionClear {
				status = MutationStatusCleared
			}
		}
		result := ValueMutationResult{Action: operation.Action, Status: status, Key: operation.Key, SourceKind: operation.sourceKind, OperationID: operation.OperationID.String()}
		if event.AfterRevision != nil {
			result.Revision = *event.AfterRevision
		}
		if expiresAt, expiryErr := repo.FindUndoExpiry(ctx, operation.OperationID); expiryErr == nil && expiresAt != nil {
			unix := expiresAt.Unix()
			result.UndoableUntil = &unix
		}
		results = append(results, result)
	}
	if found == 0 {
		return nil, false, nil
	}
	if found != len(operations) {
		return nil, false, ErrConflict
	}
	return &MutateValuesResponse{Status: "success", Operations: results}, true, nil
}
