package definition

import (
	"context"
	"fmt"

	automationdto "github.com/zgiai/ginext/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
	"gorm.io/gorm"
)

// UpdateTask replaces editable definition fields and rewrites action configuration.
func (s *service) UpdateTask(ctx context.Context, scope automationdto.TaskScope, taskID string, req automationdto.UpdateTaskRequest) (*automationdto.CreateTaskResult, error) {
	req.Timezone = normalizeScheduleTimezone(req.ScheduleType, req.Timezone)

	if err := validateUpdateTaskRequest(req); err != nil {
		return nil, err
	}

	task, err := s.taskRepo.GetByID(ctx, s.db, scope, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status == automationmodel.AutomationTaskStatusArchived {
		return nil, fmt.Errorf("automation task %s cannot be updated in status %s", task.ID, task.Status)
	}
	if task.Status == automationmodel.AutomationTaskStatusCompleted &&
		task.ScheduleType != automationmodel.AutomationScheduleTypeOnce {
		return nil, fmt.Errorf("automation task %s cannot be updated in status %s", task.ID, task.Status)
	}

	nextRunAt, err := compileNextRunAt(req.ScheduleType, req.Timezone, req.ScheduleConfig)
	if err != nil {
		return nil, err
	}

	task.Name = req.Name
	task.Description = req.Description
	task.ScheduleType = req.ScheduleType
	task.Timezone = req.Timezone
	task.ScheduleConfig = req.ScheduleConfig
	task.NextRunAt = nextRunAt
	if task.Status == automationmodel.AutomationTaskStatusCompleted {
		task.Status = automationmodel.AutomationTaskStatusActive
	}
	task.UpdatedBy = req.UpdatedBy

	actions := buildTaskActions(req.Actions)

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.taskRepo.Update(ctx, tx, task); err != nil {
			return err
		}
		if err := s.actionRepo.DeleteByTaskID(ctx, tx, task.ID); err != nil {
			return err
		}
		for _, action := range actions {
			action.TaskID = task.ID
		}
		if err := s.actionRepo.BatchCreate(ctx, tx, actions); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &automationdto.CreateTaskResult{
		Task:    task,
		Actions: actions,
	}, nil
}

func buildTaskActions(requests []automationdto.UpdateTaskActionRequest) []*automationmodel.AutomationTaskAction {
	actions := make([]*automationmodel.AutomationTaskAction, 0, len(requests))
	for index, actionReq := range requests {
		actionOrder := actionReq.ActionOrder
		if actionOrder <= 0 {
			actionOrder = index + 1
		}
		enabled := true
		if actionReq.Enabled != nil {
			enabled = *actionReq.Enabled
		}
		actions = append(actions, &automationmodel.AutomationTaskAction{
			ActionType:  actionReq.ActionType,
			ActionOrder: actionOrder,
			Enabled:     enabled,
			Config:      actionReq.Config,
		})
	}
	return actions
}
