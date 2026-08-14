package worker

import (
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const graphFlowQueueName = "graphflow"

func enqueueOrReactivateGraphFlowTask(taskManager *queue.TaskManager, task *asynq.Task, durableTaskID string) error {
	if taskManager == nil {
		return fmt.Errorf("task manager is not configured")
	}
	if _, err := taskManager.EnqueueTask(task, asynq.Queue(graphFlowQueueName)); err != nil {
		if !errors.Is(err, asynq.ErrTaskIDConflict) {
			return err
		}
		removed, resetErr := taskManager.ResetArchivedTask(graphFlowQueueName, durableTaskID)
		if resetErr != nil {
			return fmt.Errorf("reset conflicting graphflow task %s: %w", durableTaskID, resetErr)
		}
		if removed {
			if _, retryErr := taskManager.EnqueueTask(task, asynq.Queue(graphFlowQueueName)); retryErr != nil {
				return fmt.Errorf("enqueue reset graphflow task %s: %w", durableTaskID, retryErr)
			}
		}
	}
	return nil
}
