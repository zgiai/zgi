package definition

import (
	"context"
	"fmt"

	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"gorm.io/gorm"
)

// PauseTask pauses an active task and keeps its definition for later resumption.
func (s *service) PauseTask(ctx context.Context, scope automationdto.TaskScope, taskID string, actorID string) error {
	task, err := s.taskRepo.GetByID(ctx, s.db, scope, taskID)
	if err != nil {
		return err
	}
	if task.Status == automationmodel.AutomationTaskStatusArchived || task.Status == automationmodel.AutomationTaskStatusCompleted {
		return fmt.Errorf("automation task %s cannot be paused in status %s", task.ID, task.Status)
	}
	if task.Status == automationmodel.AutomationTaskStatusPaused {
		return nil
	}

	task.Status = automationmodel.AutomationTaskStatusPaused
	task.NextRunAt = nil
	task.UpdatedBy = actorID
	return s.taskRepo.Update(ctx, s.db, task)
}

// ResumeTask resumes a paused task and recalculates the next runtime.
func (s *service) ResumeTask(ctx context.Context, scope automationdto.TaskScope, taskID string, actorID string) error {
	task, err := s.taskRepo.GetByID(ctx, s.db, scope, taskID)
	if err != nil {
		return err
	}
	if task.Status == automationmodel.AutomationTaskStatusArchived || task.Status == automationmodel.AutomationTaskStatusCompleted {
		return fmt.Errorf("automation task %s cannot be resumed in status %s", task.ID, task.Status)
	}
	if task.Status == automationmodel.AutomationTaskStatusActive {
		return nil
	}

	nextRunAt, err := compileNextRunAt(task.ScheduleType, task.Timezone, task.ScheduleConfig)
	if err != nil {
		return err
	}

	task.Status = automationmodel.AutomationTaskStatusActive
	task.NextRunAt = nextRunAt
	task.UpdatedBy = actorID
	return s.taskRepo.Update(ctx, s.db, task)
}

// ArchiveTask soft-deletes a task from scheduling while preserving history.
func (s *service) ArchiveTask(ctx context.Context, scope automationdto.TaskScope, taskID string, actorID string) error {
	task, err := s.taskRepo.GetByID(ctx, s.db, scope, taskID)
	if err != nil {
		return err
	}
	if task.Status == automationmodel.AutomationTaskStatusArchived {
		return nil
	}

	task.Status = automationmodel.AutomationTaskStatusArchived
	task.NextRunAt = nil
	task.UpdatedBy = actorID
	return s.taskRepo.Update(ctx, s.db, task)
}

// DeleteTask physically deletes an archived task when MVP safety checks pass.
func (s *service) DeleteTask(ctx context.Context, scope automationdto.TaskScope, taskID string) error {
	task, err := s.taskRepo.GetByID(ctx, s.db, scope, taskID)
	if err != nil {
		return err
	}

	runCount, err := s.runRepo.CountTaskRunsByTaskID(ctx, s.db, task.ID)
	if err != nil {
		return err
	}
	if err := ValidateHardDeleteTask(task, runCount); err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.actionRepo.DeleteByTaskID(ctx, tx, task.ID); err != nil {
			return err
		}
		if err := s.taskRepo.Delete(ctx, tx, task.ID); err != nil {
			return err
		}
		return nil
	})
}
