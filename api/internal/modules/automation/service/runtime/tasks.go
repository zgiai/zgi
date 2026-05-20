package runtime

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const (
	// TypeAutomationExecute executes one persisted automation task run.
	TypeAutomationExecute = "automation:task:execute"
)

// ExecuteTaskPayload identifies the task run to execute.
type ExecuteTaskPayload struct {
	RunID string `json:"run_id"`
}

// NewExecuteTask builds the queue task used to execute one automation task run.
func NewExecuteTask(runID string, taskManager *queue.TaskManager) (*asynq.Task, error) {
	if runID == "" {
		return nil, fmt.Errorf("automation run id is empty")
	}

	payload, err := json.Marshal(ExecuteTaskPayload{RunID: runID})
	if err != nil {
		return nil, fmt.Errorf("marshal automation execute task payload: %w", err)
	}

	taskType := TypeAutomationExecute
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}

	return asynq.NewTask(taskType, payload,
		asynq.Timeout(2*time.Minute),
		asynq.MaxRetry(3),
	), nil
}
