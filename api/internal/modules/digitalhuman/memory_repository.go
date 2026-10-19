package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *GormRepository) MemoryStateForUpdate(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) (*MemoryState, error) {
	var state MemoryState
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID,
	).Take(&state).Error
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *GormRepository) CreateMemoryState(ctx context.Context, state *MemoryState) error {
	if state == nil || state.ID == uuid.Nil || state.DigitalHumanID == uuid.Nil {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Create(state).Error
}

func (r *GormRepository) UpdateMemoryStateFence(ctx context.Context, state *MemoryState, previousEpoch int64) error {
	result := r.db.WithContext(ctx).Model(&MemoryState{}).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND memory_epoch = ?",
		state.ID, state.OrganizationID, state.WorkspaceID, state.DigitalHumanID, previousEpoch,
	).Updates(map[string]any{
		"config_revision": state.ConfigRevision, "memory_epoch": state.MemoryEpoch,
		"auto_extraction_enabled": state.AutoExtractionEnabled, "extraction_cutoff_at": state.ExtractionCutoffAt,
		"updated_at": state.UpdatedAt,
	})
	if result.Error != nil {
		return fmt.Errorf("update DigitalHuman memory fence: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrMemoryConflict
	}
	return nil
}

// SetMemoryDatasetID associates the hidden projection with the source state.
// This is intentionally independent from the memory epoch: creating or
// repairing an index must not invalidate an in-flight memory mutation.
func (r *GormRepository) SetMemoryDatasetID(ctx context.Context, state *MemoryState, datasetID uuid.UUID) error {
	if state == nil || state.ID == uuid.Nil || datasetID == uuid.Nil {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&MemoryState{}).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ?",
		state.ID, state.OrganizationID, state.WorkspaceID, state.DigitalHumanID,
	).Updates(map[string]any{"memory_dataset_id": datasetID, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return fmt.Errorf("set DigitalHuman memory dataset: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrMemoryConflict
	}
	state.MemoryDatasetID = &datasetID
	return nil
}

func (r *GormRepository) MemoryByKeyForUpdate(
	ctx context.Context,
	scope Scope,
	digitalHumanID uuid.UUID,
	class memoryruntime.MemoryClass,
	scopeType memoryruntime.MemoryScopeType,
	scopeID, key string,
) (*DigitalHumanMemory, error) {
	var memory DigitalHumanMemory
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND memory_class = ? AND scope_type = ? AND scope_id = ? AND memory_key = ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, class, scopeType, scopeID, key,
	).Take(&memory).Error
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

func (r *GormRepository) MemoryByIDForUpdate(ctx context.Context, scope Scope, digitalHumanID, memoryID uuid.UUID) (*DigitalHumanMemory, error) {
	var memory DigitalHumanMemory
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ?",
		memoryID, scope.OrganizationID, scope.WorkspaceID, digitalHumanID,
	).Take(&memory).Error
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

func (r *GormRepository) CreateMemory(ctx context.Context, memory *DigitalHumanMemory) error {
	if memory == nil || memory.ID == uuid.Nil || memory.DigitalHumanID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(memory).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrMemoryConflict
		}
		return fmt.Errorf("create DigitalHuman memory: %w", err)
	}
	return nil
}

func (r *GormRepository) UpdateMemoryCAS(ctx context.Context, memory *DigitalHumanMemory, expectedRevision int64) error {
	result := r.db.WithContext(ctx).Model(&DigitalHumanMemory{}).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND revision = ?",
		memory.ID, memory.OrganizationID, memory.WorkspaceID, memory.DigitalHumanID, expectedRevision,
	).Updates(map[string]any{
		"content": memory.Content, "content_digest": memory.ContentDigest, "visibility": memory.Visibility,
		"memory_type": memory.MemoryType, "status": memory.Status, "confidence": memory.Confidence, "revision": memory.Revision,
		"config_revision": memory.ConfigRevision, "memory_epoch": memory.MemoryEpoch, "evidence": memory.Evidence,
		"source_principal_id": memory.SourcePrincipalID, "last_operation_id": memory.LastOperationID,
		"supersedes_id": memory.SupersedesID, "valid_from": memory.ValidFrom, "valid_to": memory.ValidTo,
		"updated_at": memory.UpdatedAt,
	})
	if result.Error != nil {
		return fmt.Errorf("update DigitalHuman memory: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrMemoryConflict
	}
	return nil
}

func (r *GormRepository) ListMemories(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query MemoryListQuery) ([]*DigitalHumanMemory, int64, error) {
	db := r.db.WithContext(ctx).Model(&DigitalHumanMemory{}).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ?", scope.OrganizationID, scope.WorkspaceID, digitalHumanID,
	)
	if query.Class != "" {
		db = db.Where("memory_class = ?", query.Class)
	}
	if query.Type != "" {
		db = db.Where("memory_type = ?", query.Type)
	}
	if query.ScopeType != "" {
		db = db.Where("scope_type = ?", query.ScopeType)
	}
	if query.ScopeID != "" {
		db = db.Where("scope_id = ?", query.ScopeID)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.Source != "" {
		db = db.Where("source_principal_id = ?", query.Source)
	}
	if query.Search != "" {
		like := "%" + strings.ToLower(query.Search) + "%"
		db = db.Where("LOWER(memory_key) LIKE ? OR LOWER(content) LIKE ?", like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count DigitalHuman memories: %w", err)
	}
	items := make([]*DigitalHumanMemory, 0, query.PageSize)
	if err := db.Order("updated_at DESC, id DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list DigitalHuman memories: %w", err)
	}
	if err := r.attachMemoryUndoStatus(ctx, scope, digitalHumanID, items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *GormRepository) attachMemoryUndoStatus(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, items []*DigitalHumanMemory) error {
	operations := make([]uuid.UUID, 0, len(items))
	memoryIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		memoryIDs = append(memoryIDs, item.ID)
		if item.LastOperationID != uuid.Nil {
			operations = append(operations, item.LastOperationID)
		}
	}
	if len(memoryIDs) == 0 {
		return nil
	}
	var records []MemoryUndoRecord
	if err := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND (operation_id IN ? OR memory_id IN ?)",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, operations, memoryIDs,
	).Order("created_at DESC, operation_id DESC").Find(&records).Error; err != nil {
		return fmt.Errorf("load DigitalHuman memory undo status: %w", err)
	}
	applyMemoryUndoStatus(items, records)
	return nil
}

func applyMemoryUndoStatus(items []*DigitalHumanMemory, records []MemoryUndoRecord) {
	byOperation := make(map[uuid.UUID]MemoryUndoRecord, len(records))
	latestConsumedByMemory := make(map[uuid.UUID]MemoryUndoRecord, len(records))
	for _, record := range records {
		byOperation[record.OperationID] = record
		if record.ConsumedAt != nil {
			if _, exists := latestConsumedByMemory[record.MemoryID]; !exists {
				latestConsumedByMemory[record.MemoryID] = record
			}
		}
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		record, ok := byOperation[item.LastOperationID]
		if !ok || record.MemoryID != item.ID {
			record, ok = latestConsumedByMemory[item.ID]
		}
		if !ok {
			continue
		}
		until := record.ExpiresAt
		item.UndoableUntil, item.UndoConsumed = &until, record.ConsumedAt != nil
	}
}

// ActiveMemories returns active memories for the DigitalHuman. Without a read
// scope every class and business scope is returned, so legacy business rows
// stay visible; with one, recall follows the run's source trust (ADR 0004).
func (r *GormRepository) ActiveMemories(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, limit int, readScope *memoryReadScope) ([]*DigitalHumanMemory, error) {
	db := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND status = ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, MemoryStatusActive,
	)
	if readScope != nil {
		clause, args := readScope.sqlPredicate()
		db = db.Where(clause, args...)
	}
	items := make([]*DigitalHumanMemory, 0, limit)
	if err := db.Order("updated_at DESC, id DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("retrieve DigitalHuman memories: %w", err)
	}
	return items, nil
}

func (r *GormRepository) MemoryEventByOperation(ctx context.Context, scope Scope, digitalHumanID, operationID uuid.UUID) (*MemoryEvent, error) {
	var event MemoryEvent
	err := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND operation_id = ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, operationID,
	).Take(&event).Error
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *GormRepository) CreateMemoryEvent(ctx context.Context, event *MemoryEvent) error {
	if err := r.db.WithContext(ctx).Create(event).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrMemoryConflict
		}
		return fmt.Errorf("create DigitalHuman memory event: %w", err)
	}
	return nil
}

func (r *GormRepository) CreateMemoryUndo(ctx context.Context, undo *MemoryUndoRecord) error {
	return r.db.WithContext(ctx).Create(undo).Error
}

func (r *GormRepository) MemoryUndoForUpdate(ctx context.Context, scope Scope, digitalHumanID, operationID uuid.UUID) (*MemoryUndoRecord, error) {
	var undo MemoryUndoRecord
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"operation_id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ?",
		operationID, scope.OrganizationID, scope.WorkspaceID, digitalHumanID,
	).Take(&undo).Error
	if err != nil {
		return nil, err
	}
	return &undo, nil
}

func (r *GormRepository) ConsumeMemoryUndo(ctx context.Context, operationID uuid.UUID, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&MemoryUndoRecord{}).Where("operation_id = ? AND consumed_at IS NULL", operationID).Update("consumed_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrMemoryConflict
	}
	return nil
}

func (r *GormRepository) CreateActionLedger(ctx context.Context, entry *ActionLedgerEntry) error {
	return r.db.WithContext(ctx).Create(entry).Error
}

func (r *GormRepository) CreateMemoryExtractionJob(ctx context.Context, job *MemoryExtractionJob) (bool, error) {
	if job == nil || job.ID == uuid.Nil || job.DigitalHumanID == uuid.Nil {
		return false, ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "digital_human_id"}, {Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(job)
	if result.Error != nil {
		return false, fmt.Errorf("create DigitalHuman memory extraction job: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (r *GormRepository) CancelMemoryExtractionJobs(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, memoryEpoch int64, now time.Time) error {
	return r.db.WithContext(ctx).Model(&MemoryExtractionJob{}).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND memory_epoch = ? AND status IN ?",
		scope.OrganizationID, scope.WorkspaceID, digitalHumanID, memoryEpoch,
		[]MemoryExtractionJobStatus{MemoryExtractionJobPending, MemoryExtractionJobQueued, MemoryExtractionJobFailed},
	).Updates(map[string]any{"status": "canceled", "error_code": "memory_epoch_changed", "finished_at": now, "updated_at": now}).Error
}

func (r *GormRepository) MemoryExtractionJobForUpdate(ctx context.Context, jobID uuid.UUID) (*MemoryExtractionJob, error) {
	if jobID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var job MemoryExtractionJob
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", jobID).Take(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *GormRepository) ListDueMemoryExtractionJobs(ctx context.Context, now time.Time, limit int) ([]*MemoryExtractionJob, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	items := make([]*MemoryExtractionJob, 0, limit)
	err := r.db.WithContext(ctx).Where(
		"status IN ? AND scheduled_at <= ?",
		[]MemoryExtractionJobStatus{MemoryExtractionJobPending, MemoryExtractionJobFailed}, now,
	).Order("scheduled_at ASC, id ASC").Limit(limit).Find(&items).Error
	return items, err
}

func (r *GormRepository) ClaimMemoryExtractionJob(ctx context.Context, jobID uuid.UUID, memoryEpoch int64, now time.Time) (*MemoryExtractionJob, error) {
	result := r.db.WithContext(ctx).Model(&MemoryExtractionJob{}).Where(
		"id = ? AND memory_epoch = ? AND scheduled_at <= ? AND status IN ?",
		jobID, memoryEpoch, now, []MemoryExtractionJobStatus{MemoryExtractionJobPending, MemoryExtractionJobFailed},
	).Updates(map[string]any{
		"status": MemoryExtractionJobRunning, "attempt_count": gorm.Expr("attempt_count + 1"),
		"error_code": "", "started_at": now, "finished_at": nil, "updated_at": now,
	})
	if result.Error != nil {
		return nil, fmt.Errorf("claim DigitalHuman memory extraction job: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, ErrMemoryConflict
	}
	var job MemoryExtractionJob
	if err := r.db.WithContext(ctx).Where("id = ?", jobID).Take(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *GormRepository) FinishMemoryExtractionJob(ctx context.Context, jobID uuid.UUID, status MemoryExtractionJobStatus, errorCode string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&MemoryExtractionJob{}).Where(
		"id = ? AND status = ?", jobID, MemoryExtractionJobRunning,
	).Updates(map[string]any{"status": status, "error_code": errorCode, "finished_at": now, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("finish DigitalHuman memory extraction job: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrMemoryConflict
	}
	return nil
}

func (r *GormRepository) CancelMemoryExtractionJob(ctx context.Context, jobID uuid.UUID, errorCode string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&MemoryExtractionJob{}).Where(
		"id = ? AND status IN ?", jobID,
		[]MemoryExtractionJobStatus{MemoryExtractionJobPending, MemoryExtractionJobQueued, MemoryExtractionJobFailed},
	).Updates(map[string]any{
		"status": MemoryExtractionJobCanceled, "error_code": errorCode, "finished_at": now, "updated_at": now,
	}).Error
}

func (r *GormRepository) RescheduleMemoryExtractionJob(ctx context.Context, jobID uuid.UUID, errorCode string, scheduledAt, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&MemoryExtractionJob{}).Where(
		"id = ? AND status = ?", jobID, MemoryExtractionJobRunning,
	).Updates(map[string]any{
		"status": MemoryExtractionJobFailed, "error_code": errorCode, "scheduled_at": scheduledAt,
		"finished_at": now, "updated_at": now,
	})
	if result.Error != nil {
		return fmt.Errorf("reschedule DigitalHuman memory extraction job: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrMemoryConflict
	}
	return nil
}

func (r *GormRepository) DeleteTerminalMemoryExtractionJobs(ctx context.Context, finishedBefore time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 5_000 {
		limit = 500
	}
	var ids []uuid.UUID
	if err := r.db.WithContext(ctx).Model(&MemoryExtractionJob{}).Where(
		"status IN ? AND finished_at < ?",
		[]MemoryExtractionJobStatus{MemoryExtractionJobSucceeded, MemoryExtractionJobCanceled}, finishedBefore,
	).Order("finished_at ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&MemoryExtractionJob{})
	return result.RowsAffected, result.Error
}

func recordNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
