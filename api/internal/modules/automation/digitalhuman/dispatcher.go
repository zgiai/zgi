package digitalhumantask

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	automationruntime "github.com/zgiai/zgi/api/internal/modules/automation/service/runtime"
	"github.com/zgiai/zgi/api/pkg/queue"
)

// RunDispatcher is the narrow durable-queue boundary used by task lifecycle
// code. Domain services do not need to understand Asynq task construction.
type RunDispatcher interface {
	DispatchTaskRun(context.Context, string) error
}

type queueRunDispatcher struct{ tasks *queue.TaskManager }

func NewRunDispatcher(tasks *queue.TaskManager) RunDispatcher {
	if tasks == nil {
		panic("digital human task dispatcher requires task manager")
	}
	return &queueRunDispatcher{tasks: tasks}
}

func (d *queueRunDispatcher) DispatchTaskRun(_ context.Context, runID string) error {
	task, err := automationruntime.NewExecuteTask(runID, d.tasks)
	if err != nil {
		return err
	}
	if _, err := d.tasks.EnqueueTask(task, asynq.Queue("critical")); err != nil {
		return fmt.Errorf("enqueue digital human task run: %w", err)
	}
	return nil
}
