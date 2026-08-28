package agentmemory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) WithTransaction(ctx context.Context, fn func(store) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

func (r *Repository) ResolveAgentWorkspace(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error) {
	var row struct {
		WorkspaceID uuid.UUID `gorm:"column:tenant_id"`
	}
	err := r.db.WithContext(ctx).
		Table("agents").
		Select("tenant_id").
		Where("id = ? AND deleted_at IS NULL", agentID).
		First(&row).Error
	if err != nil {
		return uuid.Nil, err
	}
	return row.WorkspaceID, nil
}

func (r *Repository) LockAgent(ctx context.Context, agentID uuid.UUID) error {
	var row struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	return r.db.WithContext(ctx).
		Table("agents").
		Select("id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND deleted_at IS NULL", agentID).
		First(&row).Error
}

func (r *Repository) ListSlots(ctx context.Context, workspaceID, agentID uuid.UUID, enabledOnly bool) ([]*AgentMemorySlot, error) {
	var slots []*AgentMemorySlot
	query := r.db.WithContext(ctx).Where("workspace_id = ? AND agent_id = ?", workspaceID, agentID)
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	err := query.Order("sort_order ASC, created_at ASC").Find(&slots).Error
	return slots, err
}

func (r *Repository) CreateSlot(ctx context.Context, slot *AgentMemorySlot) error {
	return r.db.WithContext(ctx).Create(slot).Error
}

func (r *Repository) UpdateSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID, values map[string]interface{}) (*AgentMemorySlot, error) {
	if err := r.db.WithContext(ctx).
		Model(&AgentMemorySlot{}).
		Where("workspace_id = ? AND agent_id = ? AND id = ?", workspaceID, agentID, slotID).
		Updates(values).Error; err != nil {
		return nil, err
	}
	return r.GetSlotScoped(ctx, workspaceID, agentID, slotID)
}

func (r *Repository) DeleteSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND id = ?", workspaceID, agentID, slotID).
		Delete(&AgentMemorySlot{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) GetSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID) (*AgentMemorySlot, error) {
	var slot AgentMemorySlot
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND id = ?", workspaceID, agentID, slotID).
		First(&slot).Error
	if err != nil {
		return nil, err
	}
	return &slot, nil
}

func (r *Repository) ListValuesForAgent(ctx context.Context, workspaceID, agentID uuid.UUID) ([]*AgentMemoryValue, error) {
	var values []*AgentMemoryValue
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ?", workspaceID, agentID).
		Find(&values).Error
	return values, err
}

func (r *Repository) ListValuesForUser(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) ([]*AgentMemoryValue, error) {
	var values []*AgentMemoryValue
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ?", workspaceID, agentID, userScope, userID).
		Find(&values).Error
	return values, err
}

func (r *Repository) GetValueScoped(ctx context.Context, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) (*AgentMemoryValue, error) {
	var value AgentMemoryValue
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND slot_key = ? AND user_scope = ? AND user_id = ?", workspaceID, agentID, slotKey, userScope, userID).
		First(&value).Error
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *Repository) GetValueScopedForUpdate(ctx context.Context, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) (*AgentMemoryValue, error) {
	var value AgentMemoryValue
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND agent_id = ? AND slot_key = ? AND user_scope = ? AND user_id = ?", workspaceID, agentID, slotKey, userScope, userID).
		First(&value).Error
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *Repository) UpsertValue(ctx context.Context, value *AgentMemoryValue) error {
	now := time.Now()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "workspace_id"},
			{Name: "agent_id"},
			{Name: "slot_key"},
			{Name: "user_scope"},
			{Name: "user_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"content", "revision", "source_kind", "source_conversation_id", "source_message_id",
			"source_completed_at", "extractor_version", "last_operation_id", "updated_at",
		}),
	}).Create(value).Error
}

func (r *Repository) CreateValue(ctx context.Context, value *AgentMemoryValue) error {
	return r.db.WithContext(ctx).Create(value).Error
}

func (r *Repository) UpdateValueCAS(ctx context.Context, value *AgentMemoryValue, expectedRevision int64) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&AgentMemoryValue{}).
		Where("id = ? AND workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ? AND revision = ?",
			value.ID, value.WorkspaceID, value.AgentID, value.UserScope, value.UserID, expectedRevision).
		Updates(map[string]interface{}{
			"content": value.Content, "revision": value.Revision, "source_kind": value.SourceKind,
			"source_conversation_id": value.SourceConversationID, "source_message_id": value.SourceMessageID,
			"source_completed_at": value.SourceCompletedAt, "extractor_version": value.ExtractorVersion,
			"last_operation_id": value.LastOperationID, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	value.UpdatedAt = now
	return nil
}

func (r *Repository) DeleteValueCAS(ctx context.Context, value *AgentMemoryValue, expectedRevision int64) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ? AND revision = ?",
			value.ID, value.WorkspaceID, value.AgentID, value.UserScope, value.UserID, expectedRevision).
		Delete(&AgentMemoryValue{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}

func (r *Repository) DeleteValuesForSubject(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ?", workspaceID, agentID, userScope, userID).
		Delete(&AgentMemoryValue{}).Error
}

func (r *Repository) DeleteUndoForSlot(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, slotKey string) error {
	return r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ? AND slot_key = ?", workspaceID, agentID, userScope, userID, slotKey).
		Delete(&AgentMemoryUndoRecord{}).Error
}

func (r *Repository) DeleteUndoForSubject(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ?", workspaceID, agentID, userScope, userID).
		Delete(&AgentMemoryUndoRecord{}).Error
}

func (r *Repository) CreateUndoRecord(ctx context.Context, record *AgentMemoryUndoRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *Repository) GetUndoRecordForUpdate(ctx context.Context, operationID, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (*AgentMemoryUndoRecord, error) {
	var record AgentMemoryUndoRecord
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("operation_id = ? AND workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ?", operationID, workspaceID, agentID, userScope, userID).
		First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Repository) DeleteUndoRecord(ctx context.Context, operationID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("operation_id = ?", operationID).Delete(&AgentMemoryUndoRecord{}).Error
}

func (r *Repository) FindUndoExpiry(ctx context.Context, operationID uuid.UUID) (*time.Time, error) {
	var row struct{ ExpiresAt time.Time }
	err := r.db.WithContext(ctx).Model(&AgentMemoryUndoRecord{}).
		Select("expires_at").Where("operation_id = ? AND expires_at > ?", operationID, time.Now()).Take(&row).Error
	if err != nil {
		return nil, err
	}
	return &row.ExpiresAt, nil
}

func (r *Repository) LockAgentState(ctx context.Context, workspaceID, agentID uuid.UUID) (*AgentMemoryAgentState, error) {
	seed := AgentMemoryAgentState{WorkspaceID: workspaceID, AgentID: agentID}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return nil, err
	}
	var state AgentMemoryAgentState
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND agent_id = ?", workspaceID, agentID).First(&state).Error
	return &state, err
}

func (r *Repository) UpdateAgentConfigRevision(ctx context.Context, state *AgentMemoryAgentState, scope, revision string) error {
	updates := map[string]interface{}{"updated_at": time.Now()}
	switch scope {
	case ConfigScopeDraft:
		updates["draft_config_revision"] = revision
	case ConfigScopePublished:
		updates["published_config_revision"] = revision
	default:
		return ErrInvalidInput
	}
	return r.db.WithContext(ctx).Model(&AgentMemoryAgentState{}).Where("id = ?", state.ID).Updates(updates).Error
}

func (r *Repository) CancelPendingJobsForAgentConfig(ctx context.Context, workspaceID, agentID uuid.UUID, scope, revision string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&AgentMemoryExtractionJob{}).
		Where("workspace_id = ? AND agent_id = ? AND config_scope = ? AND config_revision <> ? AND status IN ?", workspaceID, agentID, scope, revision, []string{ExtractionJobPending, ExtractionJobQueued, ExtractionJobFailed}).
		Updates(map[string]interface{}{"status": ExtractionJobCancelled, "error_code": "memory_config_changed", "finished_at": now, "updated_at": now}).Error
}

func (r *Repository) LockSubjectState(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (*AgentMemorySubjectState, error) {
	seed := AgentMemorySubjectState{WorkspaceID: workspaceID, AgentID: agentID, UserScope: userScope, UserID: userID}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return nil, err
	}
	var state AgentMemorySubjectState
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ?", workspaceID, agentID, userScope, userID).
		First(&state).Error
	return &state, err
}

func (r *Repository) UpdateSubjectEpoch(ctx context.Context, state *AgentMemorySubjectState, epoch int64) error {
	return r.db.WithContext(ctx).Model(&AgentMemorySubjectState{}).Where("id = ?", state.ID).
		Updates(map[string]interface{}{"memory_epoch": epoch, "extraction_cutoff_at": state.ExtractionCutoffAt, "updated_at": time.Now()}).Error
}

func (r *Repository) CancelPendingJobsForSubject(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&AgentMemoryExtractionJob{}).
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ? AND status IN ?", workspaceID, agentID, userScope, userID, []string{ExtractionJobPending, ExtractionJobQueued, ExtractionJobFailed}).
		Updates(map[string]interface{}{"status": ExtractionJobCancelled, "error_code": "memory_epoch_changed", "finished_at": now, "updated_at": now}).Error
}

func (r *Repository) CreateExtractionJob(ctx context.Context, job *AgentMemoryExtractionJob) error {
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *Repository) GetExtractionJob(ctx context.Context, id uuid.UUID) (*AgentMemoryExtractionJob, error) {
	var job AgentMemoryExtractionJob
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *Repository) GetExtractionJobByIdempotency(ctx context.Context, key string) (*AgentMemoryExtractionJob, error) {
	var job AgentMemoryExtractionJob
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *Repository) SupersedeConversationJobs(ctx context.Context, job *AgentMemoryExtractionJob) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&AgentMemoryExtractionJob{}).
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ? AND conversation_id = ? AND id <> ? AND status IN ?",
			job.WorkspaceID, job.AgentID, job.UserScope, job.UserID, job.ConversationID, job.ID, []string{ExtractionJobPending, ExtractionJobQueued, ExtractionJobFailed}).
		Updates(map[string]interface{}{"status": ExtractionJobCancelled, "error_code": "superseded", "finished_at": now, "updated_at": now}).Error
}

func (r *Repository) EarliestConversationForceAt(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID, conversationID uuid.UUID) (*time.Time, error) {
	var row struct{ ForceAt time.Time }
	err := r.db.WithContext(ctx).Model(&AgentMemoryExtractionJob{}).Select("force_at").
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ? AND conversation_id = ? AND status IN ?", workspaceID, agentID, userScope, userID, conversationID, []string{ExtractionJobPending, ExtractionJobQueued, ExtractionJobFailed}).
		Order("force_at ASC").Take(&row).Error
	if err != nil {
		return nil, err
	}
	return &row.ForceAt, nil
}

func (r *Repository) ClaimExtractionJob(ctx context.Context, id uuid.UUID, epoch int64) (*AgentMemoryExtractionJob, error) {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&AgentMemoryExtractionJob{}).
		Where("id = ? AND memory_epoch = ? AND status IN ?", id, epoch, []string{ExtractionJobPending, ExtractionJobQueued, ExtractionJobFailed}).
		Updates(map[string]interface{}{"status": ExtractionJobRunning, "attempt_count": gorm.Expr("attempt_count + 1"), "error_code": "", "started_at": now, "finished_at": nil, "updated_at": now})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrConflict
	}
	return r.GetExtractionJob(ctx, id)
}

func (r *Repository) FinishExtractionJob(ctx context.Context, id uuid.UUID, status, errorCode string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&AgentMemoryExtractionJob{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": status, "error_code": errorCode, "finished_at": now, "updated_at": now}).Error
}

func (r *Repository) RescheduleExtractionJob(ctx context.Context, id uuid.UUID, errorCode string, scheduledAt time.Time) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&AgentMemoryExtractionJob{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       ExtractionJobFailed,
			"error_code":   errorCode,
			"scheduled_at": scheduledAt,
			"finished_at":  now,
			"updated_at":   now,
		}).Error
}

func (r *Repository) ListDueExtractionJobs(ctx context.Context, limit int) ([]*AgentMemoryExtractionJob, error) {
	if limit <= 0 {
		limit = 100
	}
	var jobs []*AgentMemoryExtractionJob
	err := r.db.WithContext(ctx).
		Where("status IN ? AND scheduled_at <= ?", []string{ExtractionJobPending, ExtractionJobFailed}, time.Now()).
		Order("scheduled_at ASC").Limit(limit).Find(&jobs).Error
	return jobs, err
}

func (r *Repository) DeleteTerminalExtractionJobs(ctx context.Context, finishedBefore time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	var ids []uuid.UUID
	if err := r.db.WithContext(ctx).Model(&AgentMemoryExtractionJob{}).
		Where("status IN ? AND finished_at < ?", []string{ExtractionJobCompleted, ExtractionJobCancelled, ExtractionJobExhausted}, finishedBefore).
		Order("finished_at ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&AgentMemoryExtractionJob{})
	return result.RowsAffected, result.Error
}

func (r *Repository) CreateEvent(ctx context.Context, event *AgentMemoryEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *Repository) GetEventByOperationID(ctx context.Context, operationID uuid.UUID) (*AgentMemoryEvent, error) {
	var event AgentMemoryEvent
	if err := r.db.WithContext(ctx).Where("operation_id = ?", operationID).First(&event).Error; err != nil {
		return nil, err
	}
	return &event, nil
}
