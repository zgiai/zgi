package runtime

import (
	"context"
	"time"

	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationrepo "github.com/zgiai/zgi/api/internal/modules/automation/repository"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Executor executes persisted automation task runs.
type Executor struct {
	db              *gorm.DB
	taskRepo        automationrepo.TaskRepository
	actionRepo      automationrepo.ActionRepository
	runRepo         automationrepo.RunRepository
	actionExecutors *automationaction.ExecutorRegistry
}

// NewExecutor creates a runtime executor.
func NewExecutor(
	db *gorm.DB,
	actionRepo automationrepo.ActionRepository,
	runRepo automationrepo.RunRepository,
	notificationExecutor *automationaction.NotificationExecutor,
) *Executor {
	return NewExecutorWithActionExecutors(db, nil, actionRepo, runRepo, notificationExecutor)
}

// NewExecutorWithActionExecutors creates a runtime executor with a generic action executor registry.
func NewExecutorWithActionExecutors(
	db *gorm.DB,
	taskRepo automationrepo.TaskRepository,
	actionRepo automationrepo.ActionRepository,
	runRepo automationrepo.RunRepository,
	actionExecutors ...automationaction.ActionExecutor,
) *Executor {
	return &Executor{
		db:              db,
		taskRepo:        taskRepo,
		actionRepo:      actionRepo,
		runRepo:         runRepo,
		actionExecutors: automationaction.NewExecutorRegistry(actionExecutors...),
	}
}

// ExecuteTaskRun runs all enabled actions for one task run.
func (e *Executor) ExecuteTaskRun(ctx context.Context, runID string) error {
	run, err := e.runRepo.GetTaskRunByID(ctx, e.db, runID)
	if err != nil {
		return err
	}

	var task *automationmodel.AutomationTask
	if e.taskRepo != nil {
		task, err = e.taskRepo.GetByIDAnyScope(ctx, e.db, run.TaskID)
		if err != nil {
			return err
		}
	}

	if shouldCancelRunForTaskState(task, run) {
		return e.cancelTaskRun(ctx, run, cancellationReason(task.Status))
	}

	run.Status = automationmodel.AutomationTaskRunStatusRunning
	startedAt := time.Now()
	run.StartedAt = &startedAt
	if err := e.runRepo.UpdateTaskRun(ctx, e.db, run); err != nil {
		return err
	}

	actions, err := e.actionRepo.ListByTaskID(ctx, e.db, run.TaskID)
	if err != nil {
		return err
	}

	failed := false
	for _, action := range actions {
		if !action.Enabled {
			continue
		}

		actionRun := &automationmodel.AutomationActionRun{
			TaskRunID:    run.ID,
			TaskActionID: action.ID,
			ActionType:   action.ActionType,
			Status:       automationmodel.AutomationActionRunStatusSucceeded,
		}
		actionStartedAt := time.Now()
		actionRun.StartedAt = &actionStartedAt

		result, execErr := e.executeAction(ctx, task, run, action)
		if result != nil {
			actionRun.RequestPayload = result.RequestPayload
			actionRun.ResponsePayload = result.ResponsePayload
			actionRun.ChannelType = result.ChannelType
		}
		if execErr != nil {
			failed = true
			actionRun.Status = automationmodel.AutomationActionRunStatusFailed
			message := execErr.Error()
			actionRun.ErrorMessage = &message
		}

		actionFinishedAt := time.Now()
		actionRun.FinishedAt = &actionFinishedAt

		if err := e.runRepo.CreateActionRun(ctx, e.db, actionRun); err != nil {
			return err
		}
	}

	runFinishedAt := time.Now()
	run.FinishedAt = &runFinishedAt
	if failed {
		run.Status = automationmodel.AutomationTaskRunStatusFailed
		summary := "one or more automation actions failed"
		run.ErrorSummary = &summary
	} else {
		run.Status = automationmodel.AutomationTaskRunStatusSucceeded
		run.ErrorSummary = nil
	}

	if err := e.runRepo.UpdateTaskRun(ctx, e.db, run); err != nil {
		return err
	}
	if e.taskRepo != nil {
		if err := e.taskRepo.UpdateLastRun(ctx, e.db, run.TaskID, runFinishedAt, string(run.Status)); err != nil {
			logger.WarnContext(ctx, "failed to update automation task last run state",
				zap.String("task_id", run.TaskID),
				zap.String("run_id", run.ID),
				zap.String("run_status", string(run.Status)),
				zap.Error(err),
			)
		}
	}
	return nil
}

func shouldCancelRunForTaskState(task *automationmodel.AutomationTask, run *automationmodel.AutomationTaskRun) bool {
	if task == nil || run == nil {
		return false
	}
	if task.Status == automationmodel.AutomationTaskStatusArchived {
		return true
	}
	return task.Status == automationmodel.AutomationTaskStatusPaused &&
		run.TriggerSource == automationmodel.AutomationTriggerSourceScheduler
}

func cancellationReason(status automationmodel.AutomationTaskStatus) string {
	switch status {
	case automationmodel.AutomationTaskStatusPaused:
		return "automation task is paused"
	case automationmodel.AutomationTaskStatusArchived:
		return "automation task is archived"
	default:
		return "automation task is not executable"
	}
}

func (e *Executor) cancelTaskRun(ctx context.Context, run *automationmodel.AutomationTaskRun, reason string) error {
	now := time.Now()
	run.FinishedAt = &now
	run.Status = automationmodel.AutomationTaskRunStatusCancelled
	run.ErrorSummary = &reason
	return e.runRepo.UpdateTaskRun(ctx, e.db, run)
}

func (e *Executor) executeAction(
	ctx context.Context,
	task *automationmodel.AutomationTask,
	taskRun *automationmodel.AutomationTaskRun,
	action *automationmodel.AutomationTaskAction,
) (*automationaction.ActionExecutionResult, error) {
	executor, err := e.actionExecutors.Get(action.ActionType)
	if err != nil {
		return nil, err
	}

	return executor.ExecuteAction(ctx, automationaction.ActionExecutionRequest{
		Task:    task,
		TaskRun: taskRun,
		Action:  action,
	})
}
