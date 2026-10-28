package digitalhumantask

import (
	"context"
	"errors"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Reasons a server event did not start its task (ADR 0031).
const (
	ServerEventTaskInactive  = "task_inactive"
	ServerEventRunInProgress = "run_in_progress"
)

// TriggerServerEvent starts a run of an active DigitalHuman task because its
// server watch raised an alert. It does not start a second run while one is
// queued or running.
func (s *Service) TriggerServerEvent(ctx context.Context, taskID uuid.UUID, event map[string]any) (bool, string, error) {
	if s == nil || s.db == nil || taskID == uuid.Nil {
		return false, "", digitalhuman.ErrInvalidRequest
	}
	runID, reason := "", ""
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task automationmodel.AutomationTask
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND task_kind = ?", taskID.String(), automationmodel.AutomationTaskKindDigitalHuman).Take(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			reason = ServerEventTaskInactive
			return nil
		}
		if err != nil {
			return err
		}
		work := automationmodel.AutomationWorkStatus(pointerValue(task.WorkStatus))
		// A task that finished its own schedule still answers its watch.
		if (task.Status != automationmodel.AutomationTaskStatusActive && task.Status != automationmodel.AutomationTaskStatusCompleted) ||
			work == automationmodel.AutomationWorkStatusProposed || work == automationmodel.AutomationWorkStatusCanceled {
			reason = ServerEventTaskInactive
			return nil
		}
		var active int64
		if err := tx.Model(&automationmodel.AutomationTaskRun{}).Where("task_id = ? AND status IN ?", task.ID,
			[]automationmodel.AutomationTaskRunStatus{automationmodel.AutomationTaskRunStatusQueued, automationmodel.AutomationTaskRunStatusRunning}).
			Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			reason = ServerEventRunInProgress
			return nil
		}
		now := s.now()
		run := &automationmodel.AutomationTaskRun{TaskID: task.ID, TriggerSource: automationmodel.AutomationTriggerSourceServerEvent, ScheduledFor: now,
			Status: automationmodel.AutomationTaskRunStatusQueued, RuntimeContext: map[string]any{"attempt": 1, "trigger_mode": digitalhuman.TaskTriggerServerEvent, "server_event": event}}
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		value := automationmodel.AutomationWorkStatusInProgress
		task.WorkStatus, task.Status = &value, automationmodel.AutomationTaskStatusActive
		revision := task.Revision
		task.Revision++
		values, err := taskUpdateValues(&task, now)
		if err != nil {
			return err
		}
		update := tx.Model(&automationmodel.AutomationTask{}).Where("id = ? AND revision = ?", task.ID, revision).Updates(values)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return digitalhuman.ErrTaskConflict
		}
		mutation := digitalhuman.TaskMutationContext{ActorType: string(automationmodel.AutomationTaskActivityActorSystem), ActorRef: "server-watch"}
		if err := tx.Create(newActivity(task.ID, mutation, digitalhuman.TaskTriggerServerEvent, "A server watch started the task", map[string]any{"server_event": event, "task_run_id": run.ID})).Error; err != nil {
			return err
		}
		runID = run.ID
		return nil
	})
	if err != nil {
		return false, "", normalizeError(err)
	}
	if runID == "" {
		return false, reason, nil
	}
	if err := s.dispatcher.DispatchTaskRun(ctx, runID); err != nil {
		return false, "", err
	}
	return true, "", nil
}
