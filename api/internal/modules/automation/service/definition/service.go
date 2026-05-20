package definition

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	automationdto "github.com/zgiai/ginext/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
	automationrepo "github.com/zgiai/ginext/internal/modules/automation/repository"
	automationruntime "github.com/zgiai/ginext/internal/modules/automation/service/runtime"
	"github.com/zgiai/ginext/pkg/queue"
	"gorm.io/gorm"
)

// Service defines task definition and dispatch operations for automation MVP.
type Service interface {
	CreateTask(ctx context.Context, req automationdto.CreateTaskRequest) (*automationdto.CreateTaskResult, error)
	UpdateTask(ctx context.Context, scope automationdto.TaskScope, taskID string, req automationdto.UpdateTaskRequest) (*automationdto.CreateTaskResult, error)
	GetTask(ctx context.Context, scope automationdto.TaskScope, taskID string) (*automationmodel.AutomationTask, error)
	ListTasks(ctx context.Context, filter automationdto.TaskFilter) ([]*automationmodel.AutomationTask, error)
	CountTasks(ctx context.Context, filter automationdto.TaskFilter) (int64, error)
	RunTaskNow(ctx context.Context, scope automationdto.TaskScope, taskID string) (*automationmodel.AutomationTaskRun, error)
	PauseTask(ctx context.Context, scope automationdto.TaskScope, taskID string, actorID string) error
	ResumeTask(ctx context.Context, scope automationdto.TaskScope, taskID string, actorID string) error
	ArchiveTask(ctx context.Context, scope automationdto.TaskScope, taskID string, actorID string) error
	DeleteTask(ctx context.Context, scope automationdto.TaskScope, taskID string) error
	DispatchDueTasks(ctx context.Context, now time.Time, limit int) (int, error)
}

type service struct {
	db          *gorm.DB
	taskRepo    automationrepo.TaskRepository
	actionRepo  automationrepo.ActionRepository
	runRepo     automationrepo.RunRepository
	taskManager *queue.TaskManager
}

// NewService creates an automation definition service.
func NewService(
	db *gorm.DB,
	taskRepo automationrepo.TaskRepository,
	actionRepo automationrepo.ActionRepository,
	runRepo automationrepo.RunRepository,
	taskManager *queue.TaskManager,
) Service {
	return &service{
		db:          db,
		taskRepo:    taskRepo,
		actionRepo:  actionRepo,
		runRepo:     runRepo,
		taskManager: taskManager,
	}
}

// CreateTask persists a task definition together with its actions.
func (s *service) CreateTask(ctx context.Context, req automationdto.CreateTaskRequest) (*automationdto.CreateTaskResult, error) {
	req.Timezone = normalizeScheduleTimezone(req.ScheduleType, req.Timezone)

	if err := validateCreateTaskRequest(req); err != nil {
		return nil, err
	}

	nextRunAt, err := compileNextRunAt(req.ScheduleType, req.Timezone, req.ScheduleConfig)
	if err != nil {
		return nil, err
	}

	task := &automationmodel.AutomationTask{
		OrganizationID: req.OrganizationID,
		WorkspaceID:    req.WorkspaceID,
		Name:           req.Name,
		Description:    req.Description,
		Status:         automationmodel.AutomationTaskStatusActive,
		TriggerType:    automationmodel.AutomationTriggerTypeSchedule,
		ScheduleType:   req.ScheduleType,
		Timezone:       req.Timezone,
		ScheduleConfig: req.ScheduleConfig,
		NextRunAt:      nextRunAt,
		SourceType:     req.SourceType,
		SourceRef:      req.SourceRef,
		SourceSnapshot: req.SourceSnapshot,
		CreatedBy:      req.CreatedBy,
		UpdatedBy:      req.UpdatedBy,
	}

	actions := make([]*automationmodel.AutomationTaskAction, 0, len(req.Actions))
	for index, actionReq := range req.Actions {
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

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.taskRepo.Create(ctx, tx, task); err != nil {
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

// GetTask loads one task definition within scope.
func (s *service) GetTask(ctx context.Context, scope automationdto.TaskScope, taskID string) (*automationmodel.AutomationTask, error) {
	return s.taskRepo.GetByID(ctx, s.db, scope, taskID)
}

// ListTasks lists tasks within scope.
func (s *service) ListTasks(ctx context.Context, filter automationdto.TaskFilter) ([]*automationmodel.AutomationTask, error) {
	return s.taskRepo.List(ctx, s.db, filter)
}

// CountTasks counts tasks within scope.
func (s *service) CountTasks(ctx context.Context, filter automationdto.TaskFilter) (int64, error) {
	return s.taskRepo.Count(ctx, s.db, filter)
}

// DispatchDueTasks turns due task definitions into queued task runs.
func (s *service) DispatchDueTasks(ctx context.Context, now time.Time, limit int) (int, error) {
	if s.taskManager == nil {
		return 0, fmt.Errorf("automation task manager is not configured")
	}

	enqueuedRunIDs := make([]string, 0)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tasks, err := s.taskRepo.ListDueTasksForDispatch(ctx, tx, now, limit)
		if err != nil {
			return err
		}

		for _, task := range tasks {
			if task.NextRunAt == nil {
				continue
			}

			run := &automationmodel.AutomationTaskRun{
				TaskID:        task.ID,
				TriggerSource: automationmodel.AutomationTriggerSourceScheduler,
				ScheduledFor:  *task.NextRunAt,
				Status:        automationmodel.AutomationTaskRunStatusQueued,
			}
			created, err := s.runRepo.CreateTaskRunIfNotExists(ctx, tx, run)
			if err != nil {
				return err
			}

			nextRunAt, nextStatus, err := advanceTaskAfterDispatch(task)
			if err != nil {
				return err
			}
			task.NextRunAt = nextRunAt
			task.Status = nextStatus
			if err := s.taskRepo.Update(ctx, tx, task); err != nil {
				return err
			}

			if created {
				enqueuedRunIDs = append(enqueuedRunIDs, run.ID)
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	for _, runID := range enqueuedRunIDs {
		task, err := automationruntime.NewExecuteTask(runID, s.taskManager)
		if err != nil {
			return 0, err
		}
		if _, err := s.taskManager.EnqueueTask(task, asynq.Queue("critical")); err != nil {
			return 0, fmt.Errorf("enqueue automation execute task for run %s: %w", runID, err)
		}
	}

	return len(enqueuedRunIDs), nil
}
