package workflowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const WorkflowTestGenerationTaskType = "workflow_test:generate_cases"

type TaskHandlerRegistry interface {
	Register(taskType string, handler func(context.Context, *asynq.Task) error) bool
}

type GenerationTaskPayload struct {
	TaskID string `json:"task_id"`
}

func NewGenerationTaskAsynqTask(taskID string, taskManager *queue.TaskManager) (*asynq.Task, error) {
	payload, err := json.Marshal(GenerationTaskPayload{TaskID: taskID})
	if err != nil {
		return nil, fmt.Errorf("marshal workflow test generation task payload: %w", err)
	}

	taskType := WorkflowTestGenerationTaskType
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}

	return asynq.NewTask(taskType, payload, generationTaskAsynqOptions()...), nil
}

func generationTaskAsynqOptions() []asynq.Option {
	return []asynq.Option{
		asynq.Queue("default"),
		asynq.MaxRetry(0),
		asynq.Timeout(10 * time.Minute),
	}
}

func NewGenerationTaskHandler(service *Service, client llmclient.LLMClient) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if service == nil {
			return fmt.Errorf("workflow test service is not configured: %w", asynq.SkipRetry)
		}
		var payload GenerationTaskPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal workflow test generation task payload: %v: %w", err, asynq.SkipRetry)
		}
		payload.TaskID = strings.TrimSpace(payload.TaskID)
		if payload.TaskID == "" {
			return fmt.Errorf("workflow test generation task payload missing task_id: %w", asynq.SkipRetry)
		}
		return service.RunGenerationTask(ctx, payload.TaskID, client)
	}
}
