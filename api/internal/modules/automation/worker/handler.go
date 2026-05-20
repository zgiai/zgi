package worker

import (
	"context"

	"github.com/hibiken/asynq"
	automationruntime "github.com/zgiai/zgi/api/internal/modules/automation/service/runtime"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
)

// TaskHandlerRegistry avoids importing the container package directly.
type TaskHandlerRegistry interface {
	Register(taskType string, handler func(context.Context, *asynq.Task) error) bool
}

// RegisterAutomationHandlers registers automation asynq handlers.
func RegisterAutomationHandlers(
	registry TaskHandlerRegistry,
	executor *automationruntime.Executor,
	taskManager *queue.TaskManager,
) {
	if registry == nil || executor == nil {
		return
	}

	taskType := automationruntime.TypeAutomationExecute
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}

	if isNew := registry.Register(taskType, automationruntime.NewExecuteTaskHandler(executor)); isNew {
		logger.Info("Registered automation execute handler", map[string]interface{}{
			"task_type": taskType,
		})
	} else {
		logger.Warn("Automation execute handler was replaced", map[string]interface{}{
			"task_type": taskType,
		})
	}
}
