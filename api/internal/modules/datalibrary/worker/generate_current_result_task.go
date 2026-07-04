package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const TypeDataLibraryGenerateCurrentResult = "data_library:generate_current_result"

type GenerateCurrentResultPayload struct {
	ProcessingRequestID string `json:"processing_request_id"`
}

func (d *FileProcessTaskDispatcher) EnqueueGenerateCurrentResult(ctx context.Context, processingRequestID uuid.UUID) error {
	if d == nil || d.taskManager == nil {
		return nil
	}
	task, err := NewGenerateCurrentResultTask(processingRequestID, d.taskManager)
	if err != nil {
		return err
	}
	_, err = d.taskManager.EnqueueTask(task, asynq.Queue("chunking"))
	return err
}

func NewGenerateCurrentResultTask(processingRequestID uuid.UUID, taskManager *queue.TaskManager) (*asynq.Task, error) {
	if processingRequestID == uuid.Nil {
		return nil, fmt.Errorf("processing_request_id is required")
	}
	payload, err := json.Marshal(GenerateCurrentResultPayload{ProcessingRequestID: processingRequestID.String()})
	if err != nil {
		return nil, err
	}
	taskType := TypeDataLibraryGenerateCurrentResult
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	return asynq.NewTask(taskType, payload, asynq.Queue("chunking"), asynq.MaxRetry(0), asynq.Timeout(60*time.Minute)), nil
}

func RegisterGenerateCurrentResultTaskHandler(registry TaskHandlerRegistry, runner *GenerateCurrentResultRunner, taskManager *queue.TaskManager) {
	if registry == nil || runner == nil {
		return
	}
	taskType := TypeDataLibraryGenerateCurrentResult
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	registry.Register(taskType, NewGenerateCurrentResultTaskHandler(runner))
}

func NewGenerateCurrentResultTaskHandler(runner *GenerateCurrentResultRunner) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if runner == nil {
			return fmt.Errorf("data library generate current result runner is not configured: %w", asynq.SkipRetry)
		}
		var payload GenerateCurrentResultPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal data library generate current result task payload: %v: %w", err, asynq.SkipRetry)
		}
		requestID, err := uuid.Parse(payload.ProcessingRequestID)
		if err != nil || requestID == uuid.Nil {
			return fmt.Errorf("invalid processing_request_id %q: %w", payload.ProcessingRequestID, asynq.SkipRetry)
		}
		return runner.Run(ctx, requestID)
	}
}
