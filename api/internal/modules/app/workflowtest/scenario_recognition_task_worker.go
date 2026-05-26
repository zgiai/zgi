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

const WorkflowTestScenarioRecognitionTaskType = "workflow_test:recognize_scenarios"

type ScenarioRecognitionTaskPayload struct {
	TaskID string `json:"task_id"`
}

func NewScenarioRecognitionTaskAsynqTask(taskID string, taskManager *queue.TaskManager) (*asynq.Task, error) {
	payload, err := json.Marshal(ScenarioRecognitionTaskPayload{TaskID: taskID})
	if err != nil {
		return nil, fmt.Errorf("marshal workflow test scenario recognition task payload: %w", err)
	}

	taskType := WorkflowTestScenarioRecognitionTaskType
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}

	return asynq.NewTask(taskType, payload, scenarioRecognitionTaskAsynqOptions()...), nil
}

func scenarioRecognitionTaskAsynqOptions() []asynq.Option {
	return []asynq.Option{
		asynq.Queue("default"),
		asynq.MaxRetry(0),
		asynq.Timeout(10 * time.Minute),
	}
}

func NewScenarioRecognitionTaskHandler(service *Service, client llmclient.LLMClient) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if service == nil {
			return fmt.Errorf("workflow test service is not configured: %w", asynq.SkipRetry)
		}
		var payload ScenarioRecognitionTaskPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal workflow test scenario recognition task payload: %v: %w", err, asynq.SkipRetry)
		}
		payload.TaskID = strings.TrimSpace(payload.TaskID)
		if payload.TaskID == "" {
			return fmt.Errorf("workflow test scenario recognition task payload missing task_id: %w", asynq.SkipRetry)
		}
		taskRecord, err := service.repo.GetScenarioRecognitionTaskByID(ctx, payload.TaskID)
		if err != nil {
			return err
		}
		recognizer := &LLMScenarioRecognizer{
			Client:      client,
			WorkspaceID: taskRecord.WorkspaceID,
			AccountID:   taskRecord.AccountID,
			AgentID:     taskRecord.AgentID,
		}
		return service.RunScenarioRecognitionTask(ctx, payload.TaskID, recognizer)
	}
}
