package digitalhumantask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/gorm"
)

// undoKey keeps, on the activity of a change that applied without approval,
// the task as it was before the change, or undoCreated for a creation.
const (
	undoKey     = "undo"
	undoCreated = "created"
)

var _ digitalhuman.TaskUndoService = (*Service)(nil)

// keepUndo stores the task before an undoable change on that change's
// activity.
func keepUndo(changes map[string]any, mutation digitalhuman.TaskMutationContext, before *automationmodel.AutomationTask) error {
	if !mutation.Undoable {
		return nil
	}
	encoded, err := json.Marshal(before)
	if err != nil {
		return fmt.Errorf("encode task before change: %w", err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return fmt.Errorf("encode task before change: %w", err)
	}
	changes[undoKey] = snapshot
	return nil
}

// UndoTaskChange undoes the task's latest change by a person or the digital
// human, when it kept what to undo and is recent enough. Only that change is
// undone, once: an undo is itself not undoable.
func (s *Service) UndoTaskChange(ctx context.Context, access digitalhuman.TaskAccessContext, taskID uuid.UUID, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	if access.CurrentTaskID != nil {
		return nil, digitalhuman.ErrTaskPolicyRejected
	}
	if mutation.OperationID == uuid.Nil {
		mutation.OperationID = uuid.New()
	}
	if existing, err := s.taskByActivityOperation(ctx, access, mutation.OperationID); err == nil {
		if existing.ID != taskID.String() {
			return nil, digitalhuman.ErrTaskConflict
		}
		return publicTask(existing)
	}
	mutation.Undoable = false
	var undone *automationmodel.AutomationTask
	canceled := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := s.loadTask(ctx, tx, access, taskID, true)
		if err != nil {
			return err
		}
		var last automationmodel.AutomationTaskActivity
		err = tx.Where("task_id = ? AND actor_type <> ?", task.ID, automationmodel.AutomationTaskActivityActorSystem).
			Order("created_at DESC").First(&last).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return digitalhuman.ErrTaskConflict
		}
		if err != nil {
			return err
		}
		undo, ok := last.Changes[undoKey]
		now := s.now()
		if !ok || last.CreatedAt.Before(now.Add(-digitalhuman.TaskUndoWindow)) {
			return digitalhuman.ErrTaskConflict
		}
		restored := *task
		if undo == undoCreated {
			if err := applyTransition(&restored, digitalhuman.TaskActionCancel, now); err != nil {
				return err
			}
			canceled = true
		} else {
			encoded, err := json.Marshal(undo)
			if err != nil {
				return err
			}
			var before automationmodel.AutomationTask
			if err := json.Unmarshal(encoded, &before); err != nil || before.ID != task.ID {
				return digitalhuman.ErrTaskConflict
			}
			restored = before
		}
		restored.Revision = task.Revision + 1
		restored.UpdatedBy = accessActorID(access).String()
		values, err := taskUpdateValues(&restored, now)
		if err != nil {
			return err
		}
		update := tx.Model(&automationmodel.AutomationTask{}).Where("id = ? AND revision = ?", task.ID, task.Revision).Updates(values)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return digitalhuman.ErrTaskConflict
		}
		if err := tx.Create(newActivity(task.ID, mutation, "undone", "Task change undone", map[string]any{
			"undone_action": last.Action, "undone_operation_id": last.OperationID,
		})).Error; err != nil {
			return err
		}
		undone = &restored
		return nil
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	if canceled {
		_ = s.cancelActiveRuns(ctx, access, taskID)
	}
	return publicTask(undone)
}
