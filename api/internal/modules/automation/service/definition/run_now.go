package definition

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationruntime "github.com/zgiai/zgi/api/internal/modules/automation/service/runtime"
	"gorm.io/gorm"
)

// RunTaskNow creates one queued manual run for a task without changing its schedule state.
func (s *service) RunTaskNow(ctx context.Context, scope automationdto.TaskScope, taskID string) (*automationmodel.AutomationTaskRun, error) {
	if s.taskManager == nil {
		return nil, fmt.Errorf("automation task manager is not configured")
	}

	task, err := s.taskRepo.GetByID(ctx, s.db, scope, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status == automationmodel.AutomationTaskStatusArchived {
		return nil, fmt.Errorf("automation task %s cannot be manually triggered in status %s", task.ID, task.Status)
	}

	run := &automationmodel.AutomationTaskRun{
		TaskID:        task.ID,
		TriggerSource: automationmodel.AutomationTriggerSourceManualRun,
		ScheduledFor:  time.Now(),
		Status:        automationmodel.AutomationTaskRunStatusQueued,
		RuntimeContext: map[string]interface{}{
			"trigger_mode": "manual",
		},
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.runRepo.CreateTaskRun(ctx, tx, run)
	}); err != nil {
		return nil, err
	}

	executeTask, err := automationruntime.NewExecuteTask(run.ID, s.taskManager)
	if err != nil {
		return nil, err
	}
	if _, err := s.taskManager.EnqueueTask(executeTask, asynq.Queue("critical")); err != nil {
		return nil, fmt.Errorf("enqueue automation execute task for manual run %s: %w", run.ID, err)
	}

	return run, nil
}
