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

const TypeDataLibraryFileProcess = "data_library:file_process"

type FileProcessPayload struct {
	ProcessingRequestID string `json:"processing_request_id"`
}

type TaskHandlerRegistry interface {
	Register(taskType string, handler func(context.Context, *asynq.Task) error) bool
}

type FileProcessTaskDispatcher struct {
	taskManager *queue.TaskManager
}

func NewFileProcessTaskDispatcher(taskManager *queue.TaskManager) *FileProcessTaskDispatcher {
	return &FileProcessTaskDispatcher{taskManager: taskManager}
}

func (d *FileProcessTaskDispatcher) EnqueueFileProcess(ctx context.Context, processingRequestID uuid.UUID) error {
	if d == nil || d.taskManager == nil {
		return nil
	}
	task, err := NewFileProcessTask(processingRequestID, d.taskManager)
	if err != nil {
		return err
	}
	_, err = d.taskManager.EnqueueTask(task, asynq.Queue("chunking"))
	return err
}

func NewFileProcessTask(processingRequestID uuid.UUID, taskManager *queue.TaskManager) (*asynq.Task, error) {
	if processingRequestID == uuid.Nil {
		return nil, fmt.Errorf("processing_request_id is required")
	}
	payload, err := json.Marshal(FileProcessPayload{ProcessingRequestID: processingRequestID.String()})
	if err != nil {
		return nil, err
	}
	taskType := TypeDataLibraryFileProcess
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	return asynq.NewTask(taskType, payload, asynq.Queue("chunking"), asynq.MaxRetry(0), asynq.Timeout(60*time.Minute)), nil
}

func RegisterFileProcessTaskHandler(registry TaskHandlerRegistry, runner *FileProcessRunner, taskManager *queue.TaskManager) {
	if registry == nil || runner == nil {
		return
	}
	taskType := TypeDataLibraryFileProcess
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	registry.Register(taskType, NewFileProcessTaskHandler(runner))
}

func NewFileProcessTaskHandler(runner *FileProcessRunner) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if runner == nil {
			return fmt.Errorf("data library file process runner is not configured: %w", asynq.SkipRetry)
		}
		var payload FileProcessPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal data library file process task payload: %v: %w", err, asynq.SkipRetry)
		}
		requestID, err := uuid.Parse(payload.ProcessingRequestID)
		if err != nil || requestID == uuid.Nil {
			return fmt.Errorf("invalid processing_request_id %q: %w", payload.ProcessingRequestID, asynq.SkipRetry)
		}
		return runner.Run(ctx, requestID)
	}
}
