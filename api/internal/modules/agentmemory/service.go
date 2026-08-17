package agentmemory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	defaultSlotMaxChars     = 2000
	maxSlotNameChars        = 80
	maxSlotDescriptionChars = 200
	maxSlotsPerAgent        = 5
	defaultRenderBudget     = 4000
)

var (
	ErrInvalidInput = errors.New("invalid agent memory input")
	ErrNotFound     = errors.New("agent memory not found")
	ErrUnauthorized = errors.New("agent memory requester is unauthorized")
	ErrConflict     = errors.New("agent memory revision conflict")

	memoryKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	emailPattern     = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	secretPattern    = regexp.MustCompile(`(?i)\b(?:sk-[a-z0-9_\-]{16,}|gh[pousr]_[a-z0-9]{20,}|AIza[a-z0-9_\-]{20,})\b`)
	phonePattern     = regexp.MustCompile(`(?:^|\D)(?:\+?\d[\s\-]?){11}(?:\D|$)`)
)

type Service struct {
	repo store
}

func NewService(db *gorm.DB) *Service {
	return &Service{repo: NewRepository(db)}
}

type store interface {
	WithTransaction(ctx context.Context, fn func(store) error) error
	ResolveAgentWorkspace(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error)
	LockAgent(ctx context.Context, agentID uuid.UUID) error
	ListSlots(ctx context.Context, workspaceID, agentID uuid.UUID, enabledOnly bool) ([]*AgentMemorySlot, error)
	CreateSlot(ctx context.Context, slot *AgentMemorySlot) error
	UpdateSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID, values map[string]interface{}) (*AgentMemorySlot, error)
	DeleteSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID) error
	ListValuesForAgent(ctx context.Context, workspaceID, agentID uuid.UUID) ([]*AgentMemoryValue, error)
	ListValuesForUser(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) ([]*AgentMemoryValue, error)
	GetValueScoped(ctx context.Context, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) (*AgentMemoryValue, error)
	GetValueScopedForUpdate(ctx context.Context, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) (*AgentMemoryValue, error)
	UpsertValue(ctx context.Context, value *AgentMemoryValue) error
	CreateValue(ctx context.Context, value *AgentMemoryValue) error
	UpdateValueCAS(ctx context.Context, value *AgentMemoryValue, expectedRevision int64) error
	DeleteValueCAS(ctx context.Context, value *AgentMemoryValue, expectedRevision int64) error
	DeleteValuesForSubject(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) error
	DeleteUndoForSlot(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, slotKey string) error
	DeleteUndoForSubject(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) error
	CreateUndoRecord(ctx context.Context, record *AgentMemoryUndoRecord) error
	GetUndoRecordForUpdate(ctx context.Context, operationID, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (*AgentMemoryUndoRecord, error)
	DeleteUndoRecord(ctx context.Context, operationID uuid.UUID) error
	FindUndoExpiry(ctx context.Context, operationID uuid.UUID) (*time.Time, error)
	LockSubjectState(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (*AgentMemorySubjectState, error)
	ListSubjectStatesForAgentForUpdate(ctx context.Context, workspaceID, agentID uuid.UUID) ([]*AgentMemorySubjectState, error)
	UpdateSubjectEpoch(ctx context.Context, state *AgentMemorySubjectState, epoch int64) error
	CancelPendingJobsForSubject(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) error
	CreateExtractionJob(ctx context.Context, job *AgentMemoryExtractionJob) error
	GetExtractionJob(ctx context.Context, id uuid.UUID) (*AgentMemoryExtractionJob, error)
	GetExtractionJobByIdempotency(ctx context.Context, key string) (*AgentMemoryExtractionJob, error)
	SupersedeConversationJobs(ctx context.Context, job *AgentMemoryExtractionJob) error
	EarliestConversationForceAt(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID, conversationID uuid.UUID) (*time.Time, error)
	ClaimExtractionJob(ctx context.Context, id uuid.UUID, epoch int64) (*AgentMemoryExtractionJob, error)
	FinishExtractionJob(ctx context.Context, id uuid.UUID, status, errorCode string) error
	RescheduleExtractionJob(ctx context.Context, id uuid.UUID, errorCode string, scheduledAt time.Time) error
	ListDueExtractionJobs(ctx context.Context, limit int) ([]*AgentMemoryExtractionJob, error)
	DeleteTerminalExtractionJobs(ctx context.Context, finishedBefore time.Time, limit int) (int64, error)
	CreateEvent(ctx context.Context, event *AgentMemoryEvent) error
	GetEventByOperationID(ctx context.Context, operationID uuid.UUID) (*AgentMemoryEvent, error)
}

type RuntimeSlot struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	MaxChars    int    `json:"max_chars"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}

type MutationMetadata struct {
	ActorType            string
	Source               string
	SourceConversationID *uuid.UUID
	SourceMessageID      *uuid.UUID
	SourceCompletedAt    *time.Time
	SourceKind           string
	ExtractorVersion     string
	OperationID          *uuid.UUID
	ExpectedRevision     *int64
	MemoryEpoch          *int64
	EventResult          string
}

func (s *Service) ListSlots(ctx context.Context, agentID uuid.UUID) ([]SlotResponse, error) {
	workspaceID, err := s.resolveAgentWorkspace(ctx, agentID)
	if err != nil {
		return nil, err
	}
	slots, err := s.repo.ListSlots(ctx, workspaceID, agentID, false)
	if err != nil {
		return nil, fmt.Errorf("list agent memory slots: %w", err)
	}
	return slotResponses(slots), nil
}

func (s *Service) ReplaceSlots(ctx context.Context, agentID, actorID uuid.UUID, req ReplaceSlotsRequest) ([]SlotResponse, error) {
	if agentID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	if len(req.Slots) > maxSlotsPerAgent {
		return nil, fmt.Errorf("%w: too many memory slots", ErrInvalidInput)
	}

	normalized := make([]normalizedSlotInput, 0, len(req.Slots))
	seen := map[string]struct{}{}
	for i, item := range req.Slots {
		slot, err := normalizeSlotInput(item, i)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[slot.key]; ok {
			return nil, fmt.Errorf("%w: duplicate memory key %s", ErrInvalidInput, slot.key)
		}
		seen[slot.key] = struct{}{}
		normalized = append(normalized, slot)
	}

	var response []SlotResponse
	if err := s.repo.WithTransaction(ctx, func(tx store) error {
		if err := tx.LockAgent(ctx, agentID); err != nil {
			return mapRepoError(err, "lock agent")
		}
		workspaceID, err := tx.ResolveAgentWorkspace(ctx, agentID)
		if err != nil {
			return mapRepoError(err, "resolve agent workspace")
		}
		existing, err := tx.ListSlots(ctx, workspaceID, agentID, false)
		if err != nil {
			return fmt.Errorf("list existing agent memory slots: %w", err)
		}
		existingByKey := map[string]*AgentMemorySlot{}
		existingByID := map[uuid.UUID]*AgentMemorySlot{}
		for _, slot := range existing {
			if slot != nil {
				existingByKey[slot.Key] = slot
				existingByID[slot.ID] = slot
			}
		}
		now := time.Now()
		for _, input := range normalized {
			if input.id != uuid.Nil {
				current := existingByID[input.id]
				if current == nil {
					return fmt.Errorf("%w: memory item does not exist", ErrInvalidInput)
				}
				if current.Key != input.key {
					return fmt.Errorf("%w: memory key cannot be changed after creation", ErrInvalidInput)
				}
			}
			if current := existingByKey[input.key]; current != nil {
				before := *current
				updated, err := tx.UpdateSlotScoped(ctx, workspaceID, agentID, current.ID, map[string]interface{}{
					"name":        input.name,
					"description": input.description,
					"max_chars":   input.maxChars,
					"enabled":     input.enabled,
					"sort_order":  input.sortOrder,
					"updated_by":  actorID,
					"updated_at":  now,
				})
				if err != nil {
					return mapRepoError(err, "update agent memory slot")
				}
				if err := recordSlotEvent(ctx, tx, workspaceID, agentID, updated.Key, slotUpdateAction(&before, updated), organizerMetadata(), &before, updated); err != nil {
					return err
				}
				delete(existingByKey, input.key)
				continue
			}
			slot := &AgentMemorySlot{
				WorkspaceID: workspaceID,
				AgentID:     agentID,
				Key:         input.key,
				Name:        input.name,
				Description: input.description,
				MaxChars:    input.maxChars,
				Enabled:     input.enabled,
				SortOrder:   input.sortOrder,
				CreatedBy:   actorID,
				UpdatedBy:   actorID,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.CreateSlot(ctx, slot); err != nil {
				return fmt.Errorf("create agent memory slot: %w", err)
			}
			if err := recordSlotEvent(ctx, tx, workspaceID, agentID, slot.Key, EventActionSlotCreate, organizerMetadata(), nil, slot); err != nil {
				return err
			}
		}
		if len(existingByKey) > 0 {
			if err := invalidateAgentSubjects(ctx, tx, workspaceID, agentID); err != nil {
				return err
			}
		}
		for _, stale := range existingByKey {
			if stale == nil {
				continue
			}
			before := *stale
			if err := tx.DeleteSlotScoped(ctx, workspaceID, agentID, stale.ID); err != nil {
				return mapRepoError(err, "delete removed agent memory slot")
			}
			if err := recordSlotEvent(ctx, tx, workspaceID, agentID, before.Key, EventActionSlotDelete, organizerMetadata(), &before, nil); err != nil {
				return err
			}
		}
		slots, err := tx.ListSlots(ctx, workspaceID, agentID, false)
		if err != nil {
			return fmt.Errorf("list updated agent memory slots: %w", err)
		}
		response = slotResponses(slots)
		return nil
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) ReadUserMemory(ctx context.Context, workspaceID, agentID uuid.UUID, slots []RuntimeSlot, userScope string, userID uuid.UUID) ([]SlotValueResponse, error) {
	slots = normalizeRuntimeSlots(slots)
	if len(slots) == 0 {
		return []SlotValueResponse{}, nil
	}
	userScope, err := s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return nil, err
	}
	values, err := s.repo.ListValuesForUser(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, fmt.Errorf("list agent memory values: %w", err)
	}
	return runtimeSlotValueResponses(slots, values), nil
}

// ReadSubjectEpoch captures the deletion/configuration fence that automatic
// writers must present when they commit. It is deliberately read before the
// corresponding memory values so a concurrent delete cannot be missed.
func (s *Service) ReadSubjectEpoch(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (int64, error) {
	if s == nil || s.repo == nil || workspaceID == uuid.Nil || agentID == uuid.Nil {
		return 0, ErrUnauthorized
	}
	userScope, err := s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return 0, err
	}
	var epoch int64
	err = s.repo.WithTransaction(ctx, func(tx store) error {
		state, lockErr := tx.LockSubjectState(ctx, workspaceID, agentID, userScope, userID)
		if lockErr != nil {
			return lockErr
		}
		epoch = state.MemoryEpoch
		return nil
	})
	return epoch, err
}

func (s *Service) ListOrganizerValues(ctx context.Context, agentID uuid.UUID, userScope string, userID uuid.UUID) ([]SlotValueResponse, error) {
	workspaceID, err := s.resolveAgentWorkspace(ctx, agentID)
	if err != nil {
		return nil, err
	}
	userScope, err = s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return nil, err
	}
	slots, err := s.repo.ListSlots(ctx, workspaceID, agentID, false)
	if err != nil {
		return nil, fmt.Errorf("list agent memory slots: %w", err)
	}
	values, err := s.repo.ListValuesForUser(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, fmt.Errorf("list agent memory values: %w", err)
	}
	return s.enrichUndoExpiries(ctx, slotValueResponses(slots, values)), nil
}

func (s *Service) UpdateOrganizerValue(ctx context.Context, agentID uuid.UUID, userScope string, userID uuid.UUID, req UpdateValueRequest) (*SlotValueResponse, error) {
	workspaceID, err := s.resolveAgentWorkspace(ctx, agentID)
	if err != nil {
		return nil, err
	}
	slot, err := s.configuredSlotByKey(ctx, workspaceID, agentID, req.Key)
	if err != nil {
		return nil, err
	}
	return s.updateValueForSlotCompat(ctx, workspaceID, agentID, slot, userScope, userID, req, organizerMetadata())
}

func (s *Service) ClearOrganizerValue(ctx context.Context, agentID uuid.UUID, userScope string, userID uuid.UUID, key string) (*SlotValueResponse, error) {
	workspaceID, err := s.resolveAgentWorkspace(ctx, agentID)
	if err != nil {
		return nil, err
	}
	slot, err := s.configuredSlotByKey(ctx, workspaceID, agentID, key)
	if err != nil {
		return nil, err
	}
	return s.clearValueForSlot(ctx, workspaceID, agentID, slot, userScope, userID, organizerMetadata())
}

func (s *Service) UpdateValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []RuntimeSlot, userScope string, userID uuid.UUID, req UpdateValueRequest, meta MutationMetadata) (*SlotValueResponse, error) {
	key, err := normalizeKey(req.Key)
	if err != nil {
		return nil, err
	}
	slot, err := runtimeSlotByKey(slots, key)
	if err != nil {
		return nil, err
	}
	return s.updateValueForSlotCompat(ctx, workspaceID, agentID, slot, userScope, userID, req, meta)
}

func (s *Service) updateValueForSlotCompat(ctx context.Context, workspaceID, agentID uuid.UUID, slot RuntimeSlot, userScope string, userID uuid.UUID, req UpdateValueRequest, meta MutationMetadata) (*SlotValueResponse, error) {
	attempts := 1
	if req.ExpectedRevision == nil {
		attempts = 3
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		response, err := s.updateValueForSlot(ctx, workspaceID, agentID, slot, userScope, userID, req, meta)
		if err == nil || !errors.Is(err, ErrConflict) {
			return response, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func (s *Service) updateValueForSlot(ctx context.Context, workspaceID, agentID uuid.UUID, slot RuntimeSlot, userScope string, userID uuid.UUID, req UpdateValueRequest, meta MutationMetadata) (*SlotValueResponse, error) {
	key := slot.Key
	userScope, err := s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, fmt.Errorf("%w: content is required", ErrInvalidInput)
	}
	if ContainsSensitiveContent(content) {
		return nil, fmt.Errorf("%w: sensitive content cannot be saved", ErrInvalidInput)
	}
	meta = normalizeMutationMetadata(meta)

	var response *SlotValueResponse
	if err := s.repo.WithTransaction(ctx, func(tx store) error {
		if meta.SourceKind == SourceKindAutomatic {
			if meta.MemoryEpoch == nil {
				return fmt.Errorf("%w: automatic memory epoch is required", ErrInvalidInput)
			}
			state, err := tx.LockSubjectState(ctx, workspaceID, agentID, userScope, userID)
			if err != nil {
				return err
			}
			if state.MemoryEpoch != *meta.MemoryEpoch {
				return ErrConflict
			}
		}
		if len([]rune(content)) > slot.MaxChars {
			return fmt.Errorf("%w: content exceeds max_chars for %s", ErrInvalidInput, key)
		}
		before, err := tx.GetValueScopedForUpdate(ctx, workspaceID, agentID, slot.Key, userScope, userID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get agent memory value: %w", err)
		}
		if before != nil && meta.OperationID != nil && before.LastOperationID != nil && *before.LastOperationID == *meta.OperationID {
			resp := runtimeSlotValueResponse(slot, before)
			response = &resp
			return nil
		}
		if req.ExpectedRevision != nil {
			currentRevision := int64(0)
			if before != nil {
				currentRevision = before.Revision
			}
			if *req.ExpectedRevision != currentRevision {
				return ErrConflict
			}
		}
		if before != nil && meta.SourceKind == SourceKindAutomatic && meta.SourceCompletedAt != nil &&
			(before.SourceKind == SourceKindExplicit || before.SourceKind == SourceKindManager) && before.UpdatedAt.After(*meta.SourceCompletedAt) {
			return ErrConflict
		}
		nextRevision := int64(1)
		if before != nil {
			nextRevision = before.Revision + 1
		}
		value := &AgentMemoryValue{
			ID:                   uuid.New(),
			WorkspaceID:          workspaceID,
			AgentID:              agentID,
			SlotKey:              slot.Key,
			UserScope:            userScope,
			UserID:               userID,
			Content:              content,
			Revision:             nextRevision,
			SourceKind:           meta.SourceKind,
			SourceConversationID: meta.SourceConversationID,
			SourceMessageID:      meta.SourceMessageID,
			SourceCompletedAt:    meta.SourceCompletedAt,
			ExtractorVersion:     meta.ExtractorVersion,
			LastOperationID:      meta.OperationID,
		}
		if before == nil {
			if err := tx.CreateValue(ctx, value); err != nil {
				if errors.Is(err, gorm.ErrDuplicatedKey) || isDuplicateKeyError(err) {
					return ErrConflict
				}
				return fmt.Errorf("create agent memory value: %w", err)
			}
		} else {
			value.ID = before.ID
			value.CreatedAt = before.CreatedAt
			if err := tx.UpdateValueCAS(ctx, value, before.Revision); err != nil {
				return fmt.Errorf("update agent memory value: %w", err)
			}
		}
		if meta.SourceKind == SourceKindAutomatic && meta.OperationID != nil {
			record := undoRecordForAutomaticWrite(*meta.OperationID, value, before)
			if err := tx.CreateUndoRecord(ctx, record); err != nil {
				return fmt.Errorf("create agent memory undo record: %w", err)
			}
		}
		after, err := tx.GetValueScoped(ctx, workspaceID, agentID, slot.Key, userScope, userID)
		if err != nil {
			return fmt.Errorf("get updated agent memory value: %w", err)
		}
		if err := recordValueEvent(ctx, tx, workspaceID, agentID, slot.Key, userScope, userID, EventActionValueUpdate, meta, before, after); err != nil {
			return err
		}
		resp := runtimeSlotValueResponse(slot, after)
		response = &resp
		return nil
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint")
}

func (s *Service) ClearValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []RuntimeSlot, userScope string, userID uuid.UUID, key string, meta MutationMetadata) (*SlotValueResponse, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	slot, err := runtimeSlotByKey(slots, key)
	if err != nil {
		return nil, err
	}
	return s.clearValueForSlot(ctx, workspaceID, agentID, slot, userScope, userID, meta)
}

func (s *Service) clearValueForSlot(ctx context.Context, workspaceID, agentID uuid.UUID, slot RuntimeSlot, userScope string, userID uuid.UUID, meta MutationMetadata) (*SlotValueResponse, error) {
	userScope, err := s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return nil, err
	}
	var response *SlotValueResponse
	meta = normalizeMutationMetadata(meta)
	if meta.SourceKind == SourceKindAutomatic {
		return nil, fmt.Errorf("%w: automatic extraction cannot clear memory", ErrUnauthorized)
	}
	if err := s.repo.WithTransaction(ctx, func(tx store) error {
		state, err := tx.LockSubjectState(ctx, workspaceID, agentID, userScope, userID)
		if err != nil {
			return err
		}
		before, err := tx.GetValueScopedForUpdate(ctx, workspaceID, agentID, slot.Key, userScope, userID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get agent memory value: %w", err)
		}
		if before == nil {
			if meta.ExpectedRevision != nil && *meta.ExpectedRevision != 0 {
				return ErrConflict
			}
			if err := tx.DeleteUndoForSlot(ctx, workspaceID, agentID, userScope, userID, slot.Key); err != nil {
				return fmt.Errorf("delete agent memory undo records: %w", err)
			}
			if err := invalidateSubjectJobs(ctx, tx, state); err != nil {
				return err
			}
			resp := runtimeSlotValueResponse(slot, nil)
			response = &resp
			return nil
		}
		if meta.ExpectedRevision != nil && *meta.ExpectedRevision != before.Revision {
			return ErrConflict
		}
		if err := tx.DeleteValueCAS(ctx, before, before.Revision); err != nil {
			return fmt.Errorf("clear agent memory value: %w", err)
		}
		if err := tx.DeleteUndoForSlot(ctx, workspaceID, agentID, userScope, userID, slot.Key); err != nil {
			return fmt.Errorf("delete agent memory undo records: %w", err)
		}
		if err := invalidateSubjectJobs(ctx, tx, state); err != nil {
			return err
		}
		if err := recordValueEvent(ctx, tx, workspaceID, agentID, slot.Key, userScope, userID, EventActionValueClear, meta, before, nil); err != nil {
			return err
		}
		resp := runtimeSlotValueResponse(slot, nil)
		response = &resp
		return nil
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func invalidateSubjectJobs(ctx context.Context, tx store, state *AgentMemorySubjectState) error {
	if state == nil {
		return ErrInvalidInput
	}
	if err := tx.CancelPendingJobsForSubject(ctx, state.WorkspaceID, state.AgentID, state.UserScope, state.UserID); err != nil {
		return err
	}
	now := time.Now()
	if state.ExtractionCutoffAt == nil || state.ExtractionCutoffAt.Before(now) {
		state.ExtractionCutoffAt = &now
	}
	return tx.UpdateSubjectEpoch(ctx, state, state.MemoryEpoch+1)
}

func invalidateAgentSubjects(ctx context.Context, tx store, workspaceID, agentID uuid.UUID) error {
	states, err := tx.ListSubjectStatesForAgentForUpdate(ctx, workspaceID, agentID)
	if err != nil {
		return fmt.Errorf("list agent memory subject states: %w", err)
	}
	for _, state := range states {
		if state == nil {
			continue
		}
		if err := tx.CancelPendingJobsForSubject(ctx, state.WorkspaceID, state.AgentID, state.UserScope, state.UserID); err != nil {
			return err
		}
		// Configuration changes fence old writers without moving the user-delete
		// cutoff; new workers may still inspect eligible newer conversations.
		if err := tx.UpdateSubjectEpoch(ctx, state, state.MemoryEpoch+1); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ClearValuesNotInKeys(ctx context.Context, agentID uuid.UUID, keepKeys []string, invalidateSubjects bool) error {
	workspaceID, err := s.resolveAgentWorkspace(ctx, agentID)
	if err != nil {
		return err
	}
	keep := map[string]struct{}{}
	for _, key := range keepKeys {
		normalized, err := normalizeKey(key)
		if err == nil {
			keep[normalized] = struct{}{}
		}
	}
	return s.repo.WithTransaction(ctx, func(tx store) error {
		if invalidateSubjects {
			if err := invalidateAgentSubjects(ctx, tx, workspaceID, agentID); err != nil {
				return err
			}
		}
		values, err := tx.ListValuesForAgent(ctx, workspaceID, agentID)
		if err != nil {
			return fmt.Errorf("list agent memory values: %w", err)
		}
		meta := organizerMetadata()
		for _, before := range values {
			if before == nil {
				continue
			}
			if _, ok := keep[before.SlotKey]; ok {
				continue
			}
			locked, err := tx.GetValueScopedForUpdate(ctx, before.WorkspaceID, before.AgentID, before.SlotKey, before.UserScope, before.UserID)
			if err != nil {
				return err
			}
			if err := tx.DeleteValueCAS(ctx, locked, locked.Revision); err != nil {
				return fmt.Errorf("clear removed agent memory value: %w", err)
			}
			if err := tx.DeleteUndoForSlot(ctx, before.WorkspaceID, before.AgentID, before.UserScope, before.UserID, before.SlotKey); err != nil {
				return err
			}
			if err := recordValueEvent(ctx, tx, before.WorkspaceID, before.AgentID, before.SlotKey, before.UserScope, before.UserID, EventActionValueClear, meta, before, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) RenderContext(ctx context.Context, workspaceID, agentID uuid.UUID, slots []RuntimeSlot, userScope string, userID uuid.UUID, budget int) (string, error) {
	if budget <= 0 {
		budget = defaultRenderBudget
	}
	entries, err := s.ReadUserMemory(ctx, workspaceID, agentID, slots, userScope, userID)
	if err != nil {
		return "", err
	}
	return renderMemoryContext(entries, budget), nil
}

func (s *Service) resolveAgentWorkspace(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error) {
	if agentID == uuid.Nil {
		return uuid.Nil, ErrUnauthorized
	}
	workspaceID, err := s.repo.ResolveAgentWorkspace(ctx, agentID)
	if err != nil {
		return uuid.Nil, mapRepoError(err, "resolve agent workspace")
	}
	return workspaceID, nil
}

func (s *Service) resolveRuntimeScope(userScope string, userID uuid.UUID) (string, error) {
	if userID == uuid.Nil {
		return "", ErrUnauthorized
	}
	return normalizeUserScope(userScope), nil
}

func (s *Service) configuredSlotByKey(ctx context.Context, workspaceID, agentID uuid.UUID, key string) (RuntimeSlot, error) {
	normalizedKey, err := normalizeKey(key)
	if err != nil {
		return RuntimeSlot{}, err
	}
	slots, err := s.repo.ListSlots(ctx, workspaceID, agentID, false)
	if err != nil {
		return RuntimeSlot{}, fmt.Errorf("list agent memory slots: %w", err)
	}
	for _, slot := range slots {
		if slot != nil && slot.Key == normalizedKey {
			return RuntimeSlot{
				Key:         slot.Key,
				Description: slot.Description,
				MaxChars:    slot.MaxChars,
				Enabled:     slot.Enabled,
				SortOrder:   slot.SortOrder,
			}, nil
		}
	}
	return RuntimeSlot{}, fmt.Errorf("%w: memory key %s is not configured for this agent", ErrInvalidInput, normalizedKey)
}

type normalizedSlotInput struct {
	id          uuid.UUID
	key         string
	name        string
	description string
	maxChars    int
	enabled     bool
	sortOrder   int
}

func normalizeSlotInput(req SlotUpsertRequest, index int) (normalizedSlotInput, error) {
	key, err := normalizeKey(req.Key)
	if err != nil {
		return normalizedSlotInput{}, err
	}
	id := uuid.Nil
	if trimmedID := strings.TrimSpace(req.ID); trimmedID != "" {
		parsedID, err := uuid.Parse(trimmedID)
		if err != nil {
			return normalizedSlotInput{}, fmt.Errorf("%w: memory id is invalid", ErrInvalidInput)
		}
		id = parsedID
	}
	name := strings.TrimSpace(req.Name)
	if len([]rune(name)) > maxSlotNameChars {
		return normalizedSlotInput{}, fmt.Errorf("%w: name is too long for %s", ErrInvalidInput, key)
	}
	description := strings.TrimSpace(req.Description)
	if len([]rune(description)) > maxSlotDescriptionChars {
		return normalizedSlotInput{}, fmt.Errorf("%w: description is too long for %s", ErrInvalidInput, key)
	}
	maxChars := req.MaxChars
	if maxChars <= 0 {
		maxChars = defaultSlotMaxChars
	}
	if maxChars > defaultSlotMaxChars {
		return normalizedSlotInput{}, fmt.Errorf("%w: max_chars exceeds %d for %s", ErrInvalidInput, defaultSlotMaxChars, key)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sortOrder := req.SortOrder
	if sortOrder == 0 {
		sortOrder = index
	}
	return normalizedSlotInput{
		id:          id,
		key:         key,
		name:        name,
		description: description,
		maxChars:    maxChars,
		enabled:     enabled,
		sortOrder:   sortOrder,
	}, nil
}

func normalizeKey(raw string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return "", fmt.Errorf("%w: key is required", ErrInvalidInput)
	}
	if !memoryKeyPattern.MatchString(key) {
		return "", fmt.Errorf("%w: key must start with a lowercase letter and contain only lowercase letters, numbers, and underscores", ErrInvalidInput)
	}
	return key, nil
}

func normalizeUserScope(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case UserScopeEndUser:
		return UserScopeEndUser
	default:
		return UserScopeAccount
	}
}

// ClearAllValues permanently removes a subject's active values, invalidates all
// pending jobs, and advances the epoch so an already-running worker cannot recreate them.
func (s *Service) ClearAllValues(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, meta MutationMetadata) error {
	var err error
	userScope, err = s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return err
	}
	meta = normalizeMutationMetadata(meta)
	return s.repo.WithTransaction(ctx, func(tx store) error {
		state, err := tx.LockSubjectState(ctx, workspaceID, agentID, userScope, userID)
		if err != nil {
			return fmt.Errorf("lock agent memory subject: %w", err)
		}
		if err := tx.DeleteValuesForSubject(ctx, workspaceID, agentID, userScope, userID); err != nil {
			return err
		}
		if err := tx.DeleteUndoForSubject(ctx, workspaceID, agentID, userScope, userID); err != nil {
			return err
		}
		if err := tx.CancelPendingJobsForSubject(ctx, workspaceID, agentID, userScope, userID); err != nil {
			return err
		}
		cutoff := time.Now()
		if meta.SourceCompletedAt != nil {
			cutoff = *meta.SourceCompletedAt
		}
		if state.ExtractionCutoffAt == nil || state.ExtractionCutoffAt.Before(cutoff) {
			state.ExtractionCutoffAt = &cutoff
		}
		if err := tx.UpdateSubjectEpoch(ctx, state, state.MemoryEpoch+1); err != nil {
			return err
		}
		return recordEvent(ctx, tx, workspaceID, agentID, "", userScope, &userID, EventActionValuesClear, meta, nil, nil)
	})
}

func (s *Service) ExportUserMemory(ctx context.Context, workspaceID, agentID uuid.UUID, slots []RuntimeSlot, userScope string, userID uuid.UUID) (*MemoryExportResponse, error) {
	values, err := s.ReadUserMemory(ctx, workspaceID, agentID, slots, userScope, userID)
	if err != nil {
		return nil, err
	}
	values = s.enrichUndoExpiries(ctx, values)
	return &MemoryExportResponse{
		AgentID: agentID.String(), UserScope: normalizeUserScope(userScope), UserID: userID.String(),
		ExportedAt: time.Now().Unix(), Values: values,
	}, nil
}

// UndoAutomaticOperation restores exactly the value snapshot associated with an
// automatic write and only while that write is still the current revision.
func (s *Service) UndoAutomaticOperation(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID, operationID uuid.UUID, slots []RuntimeSlot) (*UndoResponse, error) {
	var err error
	userScope, err = s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return nil, err
	}
	var response *UndoResponse
	err = s.repo.WithTransaction(ctx, func(tx store) error {
		record, err := tx.GetUndoRecordForUpdate(ctx, operationID, workspaceID, agentID, userScope, userID)
		if err != nil {
			return mapRepoError(err, "get agent memory undo record")
		}
		if !record.ExpiresAt.After(time.Now()) {
			return fmt.Errorf("%w: undo window expired", ErrConflict)
		}
		current, err := tx.GetValueScopedForUpdate(ctx, workspaceID, agentID, record.SlotKey, userScope, userID)
		if err != nil {
			return mapRepoError(err, "get current agent memory value")
		}
		if current.Revision != record.ResultingRevision || current.LastOperationID == nil || *current.LastOperationID != operationID {
			return fmt.Errorf("%w: memory changed after automatic operation", ErrConflict)
		}
		var restored *AgentMemoryValue
		if !record.PreviousExists {
			if err := tx.DeleteValueCAS(ctx, current, current.Revision); err != nil {
				return err
			}
		} else {
			restored = &AgentMemoryValue{
				ID: current.ID, WorkspaceID: workspaceID, AgentID: agentID, SlotKey: record.SlotKey,
				UserScope: userScope, UserID: userID, Content: record.PreviousContent,
				Revision: current.Revision + 1, SourceKind: record.PreviousSourceKind,
				SourceConversationID: record.PreviousConversationID, SourceMessageID: record.PreviousMessageID,
				SourceCompletedAt: record.PreviousSourceCompletedAt, ExtractorVersion: record.PreviousExtractorVersion,
			}
			if err := tx.UpdateValueCAS(ctx, restored, current.Revision); err != nil {
				return err
			}
		}
		if err := tx.DeleteUndoRecord(ctx, operationID); err != nil {
			return err
		}
		meta := organizerMetadata()
		if err := recordValueEvent(ctx, tx, workspaceID, agentID, record.SlotKey, userScope, userID, EventActionValueUndo, meta, current, restored); err != nil {
			return err
		}
		response = &UndoResponse{OperationID: operationID.String()}
		if slot, slotErr := runtimeSlotByKey(slots, record.SlotKey); slotErr == nil {
			value := runtimeSlotValueResponse(slot, restored)
			response.Value = &value
		}
		return nil
	})
	return response, err
}

func (s *Service) ScheduleExtraction(ctx context.Context, req ScheduleExtractionRequest) (*AgentMemoryExtractionJob, error) {
	workspaceID, err := uuid.Parse(strings.TrimSpace(req.WorkspaceID))
	if err != nil || workspaceID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil || agentID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	userID, err := uuid.Parse(strings.TrimSpace(req.UserID))
	if err != nil || userID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(req.ConversationID))
	if err != nil || conversationID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	watermarkID, err := uuid.Parse(strings.TrimSpace(req.MessageWatermarkID))
	if err != nil || watermarkID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	userScope := normalizeUserScope(req.UserScope)
	extractorVersion := strings.TrimSpace(req.ExtractorVersion)
	if extractorVersion == "" {
		extractorVersion = "agent-memory-v2"
	}
	keyMaterial := strings.Join([]string{workspaceID.String(), agentID.String(), userScope, userID.String(), conversationID.String(), watermarkID.String(), extractorVersion}, ":")
	digest := sha256.Sum256([]byte(keyMaterial))
	idempotencyKey := hex.EncodeToString(digest[:])
	var job *AgentMemoryExtractionJob
	err = s.repo.WithTransaction(ctx, func(tx store) error {
		if existing, existingErr := tx.GetExtractionJobByIdempotency(ctx, idempotencyKey); existingErr == nil {
			job = existing
			return nil
		} else if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		state, stateErr := tx.LockSubjectState(ctx, workspaceID, agentID, userScope, userID)
		if stateErr != nil {
			return stateErr
		}
		now := time.Now()
		forceAt := now.Add(10 * time.Minute)
		if previousForce, forceErr := tx.EarliestConversationForceAt(ctx, workspaceID, agentID, userScope, userID, conversationID); forceErr == nil && previousForce != nil && previousForce.Before(forceAt) {
			forceAt = *previousForce
		} else if forceErr != nil && !errors.Is(forceErr, gorm.ErrRecordNotFound) {
			return forceErr
		}
		scheduledAt := now.Add(time.Minute)
		if forceAt.Before(scheduledAt) {
			scheduledAt = forceAt
		}
		job = &AgentMemoryExtractionJob{
			WorkspaceID: workspaceID, AgentID: agentID, UserScope: userScope, UserID: userID,
			ConversationID: conversationID, MessageWatermarkID: watermarkID, MemoryEpoch: state.MemoryEpoch,
			ExtractorVersion: extractorVersion, IdempotencyKey: idempotencyKey, Status: ExtractionJobPending,
			ScheduledAt: scheduledAt, ForceAt: forceAt,
		}
		if err := tx.CreateExtractionJob(ctx, job); err != nil {
			return err
		}
		return tx.SupersedeConversationJobs(ctx, job)
	})
	return job, err
}

func (s *Service) ListDueExtractionJobs(ctx context.Context, limit int) ([]*AgentMemoryExtractionJob, error) {
	return s.repo.ListDueExtractionJobs(ctx, limit)
}

func (s *Service) ClaimExtractionJob(ctx context.Context, id uuid.UUID) (*AgentMemoryExtractionJob, error) {
	var claimed *AgentMemoryExtractionJob
	err := s.repo.WithTransaction(ctx, func(tx store) error {
		job, err := tx.GetExtractionJob(ctx, id)
		if err != nil {
			return err
		}
		state, err := tx.LockSubjectState(ctx, job.WorkspaceID, job.AgentID, job.UserScope, job.UserID)
		if err != nil {
			return err
		}
		if state.MemoryEpoch != job.MemoryEpoch {
			_ = tx.FinishExtractionJob(ctx, id, ExtractionJobCancelled, "memory_epoch_changed")
			return ErrConflict
		}
		claimed, err = tx.ClaimExtractionJob(ctx, id, job.MemoryEpoch)
		return err
	})
	return claimed, err
}

func (s *Service) FinishExtractionJob(ctx context.Context, id uuid.UUID, status, errorCode string) error {
	return s.repo.FinishExtractionJob(ctx, id, status, errorCode)
}

func (s *Service) RescheduleExtractionJob(ctx context.Context, id uuid.UUID, errorCode string, scheduledAt time.Time) error {
	return s.repo.RescheduleExtractionJob(ctx, id, errorCode, scheduledAt)
}

func (s *Service) DeleteTerminalExtractionJobs(ctx context.Context, finishedBefore time.Time, limit int) (int64, error) {
	return s.repo.DeleteTerminalExtractionJobs(ctx, finishedBefore, limit)
}

func (s *Service) enrichUndoExpiries(ctx context.Context, values []SlotValueResponse) []SlotValueResponse {
	for i := range values {
		operationID, err := uuid.Parse(values[i].LastOperationID)
		if err != nil || operationID == uuid.Nil {
			continue
		}
		expiresAt, err := s.repo.FindUndoExpiry(ctx, operationID)
		if err != nil || expiresAt == nil {
			continue
		}
		unix := expiresAt.Unix()
		values[i].UndoableUntil = &unix
	}
	return values
}

func undoRecordForAutomaticWrite(operationID uuid.UUID, after, before *AgentMemoryValue) *AgentMemoryUndoRecord {
	record := &AgentMemoryUndoRecord{
		OperationID: operationID, WorkspaceID: after.WorkspaceID, AgentID: after.AgentID,
		UserScope: after.UserScope, UserID: after.UserID, SlotKey: after.SlotKey,
		ResultingRevision: after.Revision, ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if before != nil {
		record.PreviousExists = true
		record.PreviousContent = before.Content
		record.PreviousRevision = before.Revision
		record.PreviousSourceKind = before.SourceKind
		record.PreviousConversationID = before.SourceConversationID
		record.PreviousMessageID = before.SourceMessageID
		record.PreviousSourceCompletedAt = before.SourceCompletedAt
		record.PreviousExtractorVersion = before.ExtractorVersion
	}
	return record
}

func ContainsSensitiveContent(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return false
	}
	if emailPattern.MatchString(normalized) || secretPattern.MatchString(normalized) || phonePattern.MatchString(normalized) {
		return true
	}
	digits := 0
	for _, char := range normalized {
		if char >= '0' && char <= '9' {
			digits++
			if digits >= 12 {
				return true
			}
		} else {
			digits = 0
		}
	}
	for _, marker := range []string{
		"password", "passwd", "passcode", "credential", "secret", "api key", "apikey", "access token", "refresh token", "private key", "credit card", "bank card", "card number", "ssn", "email address", "phone number", "home address", "medical record", "health condition", "sexual orientation", "religion", "political affiliation",
		"密码", "口令", "凭据", "令牌", "秘钥", "银行卡", "信用卡", "身份证", "证件号", "验证码", "支付", "邮箱", "手机号", "电话号码", "家庭住址", "病历", "健康状况", "性取向", "宗教信仰", "政治面貌",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func normalizeRuntimeSlots(slots []RuntimeSlot) []RuntimeSlot {
	normalized := make([]RuntimeSlot, 0, len(slots))
	seen := map[string]struct{}{}
	for i, slot := range slots {
		key, err := normalizeKey(slot.Key)
		if err != nil {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		maxChars := slot.MaxChars
		if maxChars <= 0 {
			maxChars = defaultSlotMaxChars
		}
		sortOrder := slot.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		normalized = append(normalized, RuntimeSlot{
			Key:         key,
			Description: strings.TrimSpace(slot.Description),
			MaxChars:    maxChars,
			Enabled:     slot.Enabled,
			SortOrder:   sortOrder,
		})
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].SortOrder != normalized[j].SortOrder {
			return normalized[i].SortOrder < normalized[j].SortOrder
		}
		return normalized[i].Key < normalized[j].Key
	})
	return normalized
}

func runtimeSlotByKey(slots []RuntimeSlot, key string) (RuntimeSlot, error) {
	for _, slot := range normalizeRuntimeSlots(slots) {
		if slot.Key == key && slot.Enabled {
			return slot, nil
		}
	}
	return RuntimeSlot{}, fmt.Errorf("%w: memory key %s is not configured for this agent version", ErrInvalidInput, key)
}

func slotResponses(slots []*AgentMemorySlot) []SlotResponse {
	out := make([]SlotResponse, 0, len(slots))
	for _, slot := range slots {
		if slot == nil {
			continue
		}
		out = append(out, slotResponse(slot))
	}
	return out
}

func slotResponse(slot *AgentMemorySlot) SlotResponse {
	createdAt := timeFields(slot.CreatedAt)
	updatedAt := timeFields(slot.UpdatedAt)
	return SlotResponse{
		ID:               slot.ID.String(),
		Key:              slot.Key,
		Name:             slot.Name,
		Description:      slot.Description,
		MaxChars:         slot.MaxChars,
		Enabled:          slot.Enabled,
		SortOrder:        slot.SortOrder,
		CreatedAt:        createdAt.unix,
		UpdatedAt:        updatedAt.unix,
		CreatedAtUnix:    createdAt.unix,
		UpdatedAtUnix:    updatedAt.unix,
		CreatedAtISO:     createdAt.iso,
		UpdatedAtISO:     updatedAt.iso,
		CreatedAtDisplay: createdAt.display,
		UpdatedAtDisplay: updatedAt.display,
	}
}

func slotValueResponses(slots []*AgentMemorySlot, values []*AgentMemoryValue) []SlotValueResponse {
	valuesBySlot := map[string]*AgentMemoryValue{}
	for _, value := range values {
		if value != nil {
			valuesBySlot[value.SlotKey] = value
		}
	}
	out := make([]SlotValueResponse, 0, len(slots))
	for _, slot := range slots {
		if slot == nil {
			continue
		}
		out = append(out, slotValueResponse(slot, valuesBySlot[slot.Key]))
	}
	return out
}

func runtimeSlotValueResponses(slots []RuntimeSlot, values []*AgentMemoryValue) []SlotValueResponse {
	valuesBySlot := map[string]*AgentMemoryValue{}
	for _, value := range values {
		if value != nil {
			valuesBySlot[value.SlotKey] = value
		}
	}
	out := make([]SlotValueResponse, 0, len(slots))
	for _, slot := range slots {
		out = append(out, runtimeSlotValueResponse(slot, valuesBySlot[slot.Key]))
	}
	return out
}

func slotValueResponse(slot *AgentMemorySlot, value *AgentMemoryValue) SlotValueResponse {
	resp := SlotValueResponse{SlotResponse: slotResponse(slot)}
	if value != nil {
		createdAt := timeFields(value.CreatedAt)
		updatedAt := timeFields(value.UpdatedAt)
		resp.Content = value.Content
		applyValueMetadata(&resp, value)
		resp.CreatedAt = createdAt.unix
		resp.UpdatedAt = updatedAt.unix
		resp.CreatedAtUnix = createdAt.unix
		resp.UpdatedAtUnix = updatedAt.unix
		resp.CreatedAtISO = createdAt.iso
		resp.UpdatedAtISO = updatedAt.iso
		resp.CreatedAtDisplay = createdAt.display
		resp.UpdatedAtDisplay = updatedAt.display
	}
	return resp
}

func runtimeSlotValueResponse(slot RuntimeSlot, value *AgentMemoryValue) SlotValueResponse {
	resp := SlotValueResponse{
		SlotResponse: SlotResponse{
			Key:         slot.Key,
			Description: slot.Description,
			MaxChars:    slot.MaxChars,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
		},
	}
	if value != nil {
		createdAt := timeFields(value.CreatedAt)
		updatedAt := timeFields(value.UpdatedAt)
		resp.Content = value.Content
		applyValueMetadata(&resp, value)
		resp.CreatedAt = createdAt.unix
		resp.UpdatedAt = updatedAt.unix
		resp.CreatedAtUnix = createdAt.unix
		resp.UpdatedAtUnix = updatedAt.unix
		resp.CreatedAtISO = createdAt.iso
		resp.UpdatedAtISO = updatedAt.iso
		resp.CreatedAtDisplay = createdAt.display
		resp.UpdatedAtDisplay = updatedAt.display
	}
	return resp
}

type responseTimeFields struct {
	unix    int64
	iso     string
	display string
}

func timeFields(value time.Time) responseTimeFields {
	if value.IsZero() {
		return responseTimeFields{}
	}
	utc := value.UTC()
	return responseTimeFields{
		unix:    utc.Unix(),
		iso:     utc.Format(time.RFC3339),
		display: utc.Format("2006-01-02 15:04:05 UTC"),
	}
}

func renderMemoryContext(entries []SlotValueResponse, budget int) string {
	if len(entries) == 0 {
		return ""
	}
	var builder strings.Builder
	header := "Agent memory is enabled for this agent and current user.\nOnly use the listed memory keys. Do not invent new keys or temporary memories.\n\nAvailable memory items:\n"
	if len(header) > budget {
		return ""
	}
	builder.WriteString(header)
	for _, entry := range entries {
		line := fmt.Sprintf("- key: %s\n  description: %s\n  max_chars: %d\n  content: %s\n",
			entry.Key,
			strings.TrimSpace(entry.Description),
			entry.MaxChars,
			strings.TrimSpace(entry.Content),
		)
		if builder.Len()+len(line) > budget {
			break
		}
		builder.WriteString(line)
	}
	return strings.TrimSpace(builder.String())
}

func organizerMetadata() MutationMetadata {
	now := time.Now()
	return MutationMetadata{ActorType: EventActorOrganizer, Source: EventSourceAPI, SourceKind: SourceKindManager, SourceCompletedAt: &now}
}

func applyValueMetadata(resp *SlotValueResponse, value *AgentMemoryValue) {
	if resp == nil || value == nil {
		return
	}
	resp.Revision = value.Revision
	resp.SourceKind = value.SourceKind
	resp.ExtractorVersion = value.ExtractorVersion
	if value.SourceConversationID != nil {
		resp.SourceConversationID = value.SourceConversationID.String()
	}
	if value.SourceMessageID != nil {
		resp.SourceMessageID = value.SourceMessageID.String()
	}
	if value.SourceCompletedAt != nil {
		resp.SourceCompletedAt = value.SourceCompletedAt.Unix()
	}
	if value.LastOperationID != nil {
		resp.LastOperationID = value.LastOperationID.String()
	}
}

func modelMetadata(conversationID *string, messageID *string) MutationMetadata {
	meta := MutationMetadata{ActorType: EventActorModel, Source: EventSourceAgent}
	if conversationID != nil {
		if id, err := uuid.Parse(*conversationID); err == nil {
			meta.SourceConversationID = &id
		}
	}
	if messageID != nil {
		if id, err := uuid.Parse(*messageID); err == nil {
			meta.SourceMessageID = &id
		}
	}
	return meta
}

func normalizeMutationMetadata(meta MutationMetadata) MutationMetadata {
	if meta.ActorType == "" {
		meta.ActorType = EventActorSystem
	}
	if meta.Source == "" {
		meta.Source = EventSourceAPI
	}
	if meta.SourceKind == "" {
		if meta.ActorType == EventActorModel {
			meta.SourceKind = SourceKindExplicit
		} else {
			meta.SourceKind = SourceKindManager
		}
	}
	if meta.SourceCompletedAt == nil {
		now := time.Now()
		meta.SourceCompletedAt = &now
	}
	if meta.SourceKind == SourceKindAutomatic && meta.OperationID == nil {
		operationID := uuid.New()
		meta.OperationID = &operationID
	}
	return meta
}

func slotUpdateAction(before *AgentMemorySlot, after *AgentMemorySlot) string {
	if before != nil && after != nil && before.Enabled && !after.Enabled {
		return EventActionSlotDisable
	}
	return EventActionSlotUpdate
}

func recordSlotEvent(ctx context.Context, repo store, workspaceID, agentID uuid.UUID, slotKey string, action string, meta MutationMetadata, before *AgentMemorySlot, after *AgentMemorySlot) error {
	return recordEvent(ctx, repo, workspaceID, agentID, slotKey, "", nil, action, meta, nil, nil)
}

func recordValueEvent(ctx context.Context, repo store, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID, action string, meta MutationMetadata, before *AgentMemoryValue, after *AgentMemoryValue) error {
	var beforeRevision, afterRevision *int64
	if before != nil {
		value := before.Revision
		beforeRevision = &value
	}
	if after != nil {
		value := after.Revision
		afterRevision = &value
	}
	return recordEvent(ctx, repo, workspaceID, agentID, slotKey, userScope, &userID, action, meta, beforeRevision, afterRevision)
}

func recordEvent(ctx context.Context, repo store, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID *uuid.UUID, action string, meta MutationMetadata, beforeRevision, afterRevision *int64) error {
	meta = normalizeMutationMetadata(meta)
	event := &AgentMemoryEvent{
		OperationID:          meta.OperationID,
		WorkspaceID:          workspaceID,
		AgentID:              agentID,
		SlotKey:              slotKey,
		UserScope:            userScope,
		UserID:               userID,
		Action:               action,
		ActorType:            meta.ActorType,
		Source:               meta.Source,
		SourceConversationID: meta.SourceConversationID,
		SourceMessageID:      meta.SourceMessageID,
		BeforeRevision:       beforeRevision,
		AfterRevision:        afterRevision,
		Result:               strings.TrimSpace(meta.EventResult),
	}
	if event.Result == "" {
		event.Result = "success"
	}
	if err := repo.CreateEvent(ctx, event); err != nil {
		return fmt.Errorf("record agent memory event: %w", err)
	}
	return nil
}

func mapRepoError(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", message, err)
}

func sortSlotResponses(slots []SlotResponse) {
	sort.SliceStable(slots, func(i, j int) bool {
		if slots[i].SortOrder != slots[j].SortOrder {
			return slots[i].SortOrder < slots[j].SortOrder
		}
		return slots[i].Key < slots[j].Key
	})
}
