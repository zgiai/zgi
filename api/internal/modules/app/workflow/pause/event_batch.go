package pause

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maximumEventBatchCount = 64
	maximumEventBatchBytes = 1024 * 1024
)

type batchRunScope struct {
	found                  bool
	runtimeProtocolVersion int
	nextEventSequence      int
	activeExecutionID      string
}

type preparedEventDraft struct {
	draft     EventDraft
	eventJSON string
	resultAt  int
}

// AppendEventBatch persists a batch in one transaction. Events become visible
// to callers only after the transaction commits.
func (s *Service) AppendEventBatch(ctx context.Context, request AppendEventBatchRequest) ([]StoredEvent, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	var stored []StoredEvent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		stored, err = s.AppendEventBatchTx(ctx, tx, request)
		return err
	})
	if err != nil {
		return nil, err
	}
	return stored, nil
}

// AppendEventPayloadTx is the single-event compatibility adapter for callers
// that already own a transaction.
func (s *Service) AppendEventPayloadTx(ctx context.Context, tx *gorm.DB, params AppendEventParams) (*RunEventPayload, error) {
	stored, err := s.AppendEventBatchTx(ctx, tx, eventBatchRequestFromAppendParams(params))
	if err != nil {
		return nil, err
	}
	if len(stored) != 1 || stored[0].Payload == nil {
		return nil, fmt.Errorf("workflow event batch returned no event")
	}
	return stored[0].Payload, nil
}

// AppendEventBatchTx persists a batch using the caller's transaction. The
// caller must not publish returned events until that transaction commits.
func (s *Service) AppendEventBatchTx(ctx context.Context, tx *gorm.DB, request AppendEventBatchRequest) ([]StoredEvent, error) {
	chunks, err := splitEventDrafts(request.Events)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return []StoredEvent{}, nil
	}
	stored := make([]StoredEvent, 0, len(request.Events))
	for _, events := range chunks {
		chunkRequest := request
		chunkRequest.Events = events
		chunkStored, appendErr := s.appendEventBatchChunkTx(ctx, tx, chunkRequest)
		if appendErr != nil {
			return nil, appendErr
		}
		stored = append(stored, chunkStored...)
	}
	return stored, nil
}

func (s *Service) appendEventBatchChunkTx(ctx context.Context, tx *gorm.DB, request AppendEventBatchRequest) ([]StoredEvent, error) {
	if s == nil || tx == nil {
		return nil, fmt.Errorf("workflow event batch persistence is not initialized")
	}
	if strings.TrimSpace(request.WorkflowRunID) == "" {
		return nil, fmt.Errorf("workflow run id is empty")
	}
	if len(request.Events) == 0 {
		return []StoredEvent{}, nil
	}
	prepared, payloadBytes, err := prepareEventBatch(request.Events)
	if err != nil {
		return nil, err
	}
	if payloadBytes > maximumEventBatchBytes {
		return nil, fmt.Errorf("workflow event batch exceeds %d bytes", maximumEventBatchBytes)
	}

	lockStartedAt := time.Now()
	scope, unlock, err := lockEventBatchRun(ctx, tx, request)
	recordEventRunLockWait(ctx, time.Since(lockStartedAt))
	if err != nil {
		return nil, err
	}
	if unlock != nil {
		defer unlock()
	}
	if err := validateExpectedPauseEventOwner(ctx, tx, appendParamsForBatchFence(request)); err != nil {
		return nil, err
	}

	results := make([]StoredEvent, len(request.Events))
	existing, err := loadIdempotentBatchEvents(ctx, tx, request.WorkflowRunID, request.Events)
	if err != nil {
		return nil, err
	}

	newDrafts := make([]preparedEventDraft, 0, len(prepared))
	batchSeen := make(map[string]int, len(prepared))
	for index, item := range prepared {
		key := strings.TrimSpace(item.draft.IdempotencyKey)
		if key != "" {
			if event, ok := existing[key]; ok {
				payload, payloadErr := runEventPayloadFromModel(event)
				if payloadErr != nil {
					return nil, payloadErr
				}
				results[index] = StoredEvent{Payload: payload, Inserted: false}
				continue
			}
			if firstIndex, ok := batchSeen[key]; ok {
				item.resultAt = firstIndex
				newDrafts = append(newDrafts, item)
				continue
			}
			batchSeen[key] = index
		}
		item.resultAt = index
		newDrafts = append(newDrafts, item)
	}

	uniqueDrafts := make([]preparedEventDraft, 0, len(newDrafts))
	uniqueResultIndexes := make(map[string]int, len(newDrafts))
	for _, item := range newDrafts {
		key := strings.TrimSpace(item.draft.IdempotencyKey)
		if key != "" {
			if _, ok := uniqueResultIndexes[key]; ok {
				continue
			}
			uniqueResultIndexes[key] = item.resultAt
		}
		uniqueDrafts = append(uniqueDrafts, item)
	}

	if len(uniqueDrafts) > 0 {
		flushReason := strings.TrimSpace(request.FlushReason)
		if flushReason == "" {
			flushReason = "immediate"
		}
		startSequence, err := allocateEventBatchSequenceRange(tx, request.WorkflowRunID, scope, len(uniqueDrafts), request.Fence)
		if err != nil {
			return nil, err
		}
		recordEventSequenceAllocation(ctx)
		now := time.Now()
		models := make([]RunEvent, 0, len(uniqueDrafts))
		for offset, item := range uniqueDrafts {
			draft := item.draft
			if draft.Category == "" {
				draft.Category = eventCategory(draft.EventType)
			}
			if draft.OccurredAt.IsZero() {
				draft.OccurredAt = now
			}
			if draft.SchemaVersion <= 0 {
				if scope.found && scope.runtimeProtocolVersion >= 2 {
					draft.SchemaVersion = 2
				} else {
					draft.SchemaVersion = 1
				}
			}
			if draft.ExecutionID == "" {
				draft.ExecutionID = scope.activeExecutionID
			}
			models = append(models, RunEvent{
				ID:              uuid.NewString(),
				TenantID:        request.TenantID,
				AppID:           request.AppID,
				WorkflowRunID:   request.WorkflowRunID,
				Sequence:        startSequence + offset,
				EventType:       draft.EventType,
				EventData:       item.eventJSON,
				CreatedAt:       now,
				SchemaVersion:   draft.SchemaVersion,
				Category:        draft.Category,
				ExecutionID:     nullableString(draft.ExecutionID),
				PauseID:         nullableString(draft.PauseID),
				PauseGeneration: draft.PauseGeneration,
				IdempotencyKey:  nullableString(draft.IdempotencyKey),
				OccurredAt:      draft.OccurredAt,
			})
		}
		if err := tx.WithContext(ctx).Create(&models).Error; err != nil {
			return nil, fmt.Errorf("create workflow event batch: %w", err)
		}
		for index, model := range models {
			payload, payloadErr := runEventPayloadFromModel(model)
			if payloadErr != nil {
				return nil, payloadErr
			}
			resultIndex := uniqueDrafts[index].resultAt
			results[resultIndex] = StoredEvent{Payload: payload, Inserted: true}
		}
		for index, item := range prepared {
			if results[index].Payload != nil || item.draft.IdempotencyKey == "" {
				continue
			}
			firstIndex, ok := batchSeen[item.draft.IdempotencyKey]
			if !ok || results[firstIndex].Payload == nil {
				continue
			}
			results[index] = StoredEvent{Payload: results[firstIndex].Payload, Inserted: false}
		}
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_notify('workflow_runtime_events', ?)", request.WorkflowRunID).Error; err != nil {
				return nil, fmt.Errorf("notify workflow event subscribers: %w", err)
			}
			recordEventNotify(ctx, len(models), flushReason)
		}
		for _, model := range models {
			recordRuntimeEventWritten(ctx, model.Category)
		}
	}

	flushReason := strings.TrimSpace(request.FlushReason)
	if flushReason == "" {
		flushReason = "immediate"
	}
	recordEventBatch(ctx, len(request.Events), payloadBytes, len(uniqueDrafts), flushReason)
	return results, nil
}

func splitEventDrafts(events []EventDraft) ([][]EventDraft, error) {
	if len(events) == 0 {
		return nil, nil
	}
	chunks := make([][]EventDraft, 0, (len(events)+maximumEventBatchCount-1)/maximumEventBatchCount)
	current := make([]EventDraft, 0, maximumEventBatchCount)
	currentBytes := 0
	for index, event := range events {
		raw, err := json.Marshal(event.EventData)
		if err != nil {
			return nil, fmt.Errorf("marshal workflow event %d data: %w", index, err)
		}
		if len(raw) > maximumEventBatchBytes {
			return nil, fmt.Errorf("workflow event %d exceeds %d bytes", index, maximumEventBatchBytes)
		}
		if len(current) > 0 && (len(current) >= maximumEventBatchCount || currentBytes+len(raw) > maximumEventBatchBytes) {
			chunks = append(chunks, current)
			current = make([]EventDraft, 0, maximumEventBatchCount)
			currentBytes = 0
		}
		current = append(current, event)
		currentBytes += len(raw)
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks, nil
}

func prepareEventBatch(events []EventDraft) ([]preparedEventDraft, int, error) {
	prepared := make([]preparedEventDraft, 0, len(events))
	totalBytes := 0
	for index, draft := range events {
		if strings.TrimSpace(draft.EventType) == "" {
			return nil, 0, fmt.Errorf("workflow event %d type is empty", index)
		}
		raw, err := json.Marshal(draft.EventData)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal workflow event %d data: %w", index, err)
		}
		totalBytes += len(raw)
		prepared = append(prepared, preparedEventDraft{draft: draft, eventJSON: string(raw), resultAt: index})
	}
	return prepared, totalBytes, nil
}

func lockEventBatchRun(ctx context.Context, tx *gorm.DB, request AppendEventBatchRequest) (batchRunScope, func(), error) {
	var run struct {
		RuntimeProtocolVersion int     `gorm:"column:runtime_protocol_version"`
		NextEventSequence      int     `gorm:"column:next_event_sequence"`
		ExecutionGeneration    int64   `gorm:"column:execution_generation"`
		ActiveExecutionID      *string `gorm:"column:active_execution_id"`
	}
	err := tx.WithContext(ctx).Table("workflow_run_logs").Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("runtime_protocol_version, next_event_sequence, execution_generation, active_execution_id").
		Where("id = ?", request.WorkflowRunID).Take(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && request.Fence.ExpectedExecutionID == "" {
		lockValue, _ := runEventLocks.LoadOrStore(request.WorkflowRunID, &sync.Mutex{})
		legacy := lockValue.(*sync.Mutex)
		legacy.Lock()
		return batchRunScope{}, legacy.Unlock, nil
	}
	if err != nil {
		return batchRunScope{}, nil, fmt.Errorf("lock workflow run for event batch: %w", err)
	}
	activeExecutionID := stringValue(run.ActiveExecutionID)
	if request.Fence.ExpectedExecutionID != "" && request.Fence.ExpectedExecutionGeneration > 0 &&
		(activeExecutionID != request.Fence.ExpectedExecutionID || run.ExecutionGeneration != request.Fence.ExpectedExecutionGeneration) {
		recordStaleAppendRejected(ctx, "execution_owner")
		return batchRunScope{}, nil, ErrExecutionOwnershipLost
	}
	return batchRunScope{
		found:                  true,
		runtimeProtocolVersion: run.RuntimeProtocolVersion,
		nextEventSequence:      run.NextEventSequence,
		activeExecutionID:      activeExecutionID,
	}, nil, nil
}

func loadIdempotentBatchEvents(ctx context.Context, tx *gorm.DB, workflowRunID string, events []EventDraft) (map[string]RunEvent, error) {
	keys := make([]string, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		key := strings.TrimSpace(event.IdempotencyKey)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return map[string]RunEvent{}, nil
	}
	var records []RunEvent
	if err := tx.WithContext(ctx).Where("workflow_run_id = ? AND idempotency_key IN ?", workflowRunID, keys).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("load idempotent workflow event batch: %w", err)
	}
	result := make(map[string]RunEvent, len(records))
	for _, record := range records {
		if record.IdempotencyKey != nil {
			result[*record.IdempotencyKey] = record
		}
	}
	return result, nil
}

func allocateEventBatchSequenceRange(tx *gorm.DB, workflowRunID string, scope batchRunScope, count int, fence RuntimeFence) (int, error) {
	if count <= 0 {
		return 0, nil
	}
	if !scope.found {
		lastSequence, err := allocateLegacyRunEventSequence(tx, workflowRunID)
		if err != nil {
			return 0, err
		}
		return lastSequence, nil
	}
	query := tx.Table("workflow_run_logs").Where("id = ? AND next_event_sequence = ?", workflowRunID, scope.nextEventSequence)
	if fence.ExpectedExecutionID != "" && fence.ExpectedExecutionGeneration > 0 {
		query = query.Where("active_execution_id = ? AND execution_generation = ?", fence.ExpectedExecutionID, fence.ExpectedExecutionGeneration)
	}
	result := query.UpdateColumn("next_event_sequence", gorm.Expr("next_event_sequence + ?", count))
	if result.Error != nil {
		return 0, fmt.Errorf("allocate workflow event sequence range: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		if fence.ExpectedExecutionID != "" {
			return 0, ErrExecutionOwnershipLost
		}
		return 0, fmt.Errorf("workflow event sequence range was not allocated")
	}
	return scope.nextEventSequence + 1, nil
}

func appendParamsForBatchFence(request AppendEventBatchRequest) AppendEventParams {
	return AppendEventParams{
		WorkflowRunID:               request.WorkflowRunID,
		ExpectedExecutionID:         request.Fence.ExpectedExecutionID,
		ExpectedExecutionGeneration: request.Fence.ExpectedExecutionGeneration,
		ExpectedPauseID:             request.Fence.ExpectedPauseID,
		ExpectedPauseGeneration:     request.Fence.ExpectedPauseGeneration,
		ExpectedPauseRevision:       request.Fence.ExpectedPauseRevision,
	}
}

func eventDraftFromAppendParams(params AppendEventParams) EventDraft {
	return EventDraft{
		EventType: params.EventType, EventData: params.EventData, SchemaVersion: params.SchemaVersion,
		Category: params.Category, ExecutionID: params.ExecutionID, PauseID: params.PauseID,
		PauseGeneration: params.PauseGeneration, IdempotencyKey: params.IdempotencyKey, OccurredAt: params.OccurredAt,
	}
}

func eventBatchRequestFromAppendParams(params AppendEventParams) AppendEventBatchRequest {
	return AppendEventBatchRequest{
		TenantID: params.TenantID, AppID: params.AppID, WorkflowRunID: params.WorkflowRunID,
		FlushReason: "single",
		Fence: RuntimeFence{
			ExpectedExecutionID: params.ExpectedExecutionID, ExpectedExecutionGeneration: params.ExpectedExecutionGeneration,
			ExpectedPauseID: params.ExpectedPauseID, ExpectedPauseGeneration: params.ExpectedPauseGeneration,
			ExpectedPauseRevision: params.ExpectedPauseRevision,
		},
		Events: []EventDraft{eventDraftFromAppendParams(params)},
	}
}
