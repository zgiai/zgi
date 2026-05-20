package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	// TaskTypeDispatchDue scans the automation task table and enqueues due runs.
	TaskTypeDispatchDue = "automation:dispatch:due"
)

// DispatchDueTask is the fixed scheduler task that scans due automation tasks.
type DispatchDueTask struct {
	batchSize int
}

// NewDispatchDueTask creates a periodic dispatcher task.
func NewDispatchDueTask(batchSize int) *DispatchDueTask {
	if batchSize <= 0 {
		batchSize = 200
	}
	return &DispatchDueTask{batchSize: batchSize}
}

// TaskType returns the task type identifier.
func (t *DispatchDueTask) TaskType() string {
	return TaskTypeDispatchDue
}

// CronSpec returns the periodic schedule used by the MVP dispatcher.
func (t *DispatchDueTask) CronSpec() string {
	return "* * * * *"
}

// Interval returns zero because the task uses CronSpec.
func (t *DispatchDueTask) Interval() time.Duration {
	return 0
}

// Payload returns no payload because the dispatcher uses static configuration.
func (t *DispatchDueTask) Payload() []byte {
	return nil
}

// Options returns scheduler task options.
func (t *DispatchDueTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.Queue("scheduler"),
		asynq.MaxRetry(1),
		asynq.Timeout(30 * time.Second),
		asynq.Unique(50 * time.Second),
	}
}

// DispatchDueHandler executes one dispatcher sweep.
type DispatchDueHandler struct {
	service   automationdefinition.Service
	batchSize int
}

// NewDispatchDueHandler creates a dispatcher handler.
func NewDispatchDueHandler(service automationdefinition.Service, batchSize int) *DispatchDueHandler {
	if batchSize <= 0 {
		batchSize = 200
	}
	return &DispatchDueHandler{
		service:   service,
		batchSize: batchSize,
	}
}

// Handle performs one due-task dispatch sweep.
func (h *DispatchDueHandler) Handle(ctx context.Context, task *asynq.Task) error {
	_ = task

	if h.service == nil {
		err := fmt.Errorf("automation definition service is not configured")
		logger.CriticalContext(ctx, "automation dispatch handler is not configured", err)
		return err
	}

	count, err := h.service.DispatchDueTasks(ctx, time.Now(), h.batchSize)
	if err != nil {
		logger.CriticalContext(ctx, "automation dispatch sweep failed", err, "batch_size", h.batchSize)
		return err
	}

	fields := map[string]interface{}{
		"enqueued_runs": count,
		"batch_size":    h.batchSize,
	}
	if count == 0 {
		logger.Debug("automation dispatch sweep completed", fields)
		return nil
	}

	logger.Info("automation dispatch sweep completed", fields)
	return nil
}
