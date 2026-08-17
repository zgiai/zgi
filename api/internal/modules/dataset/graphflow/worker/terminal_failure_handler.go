package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// withDurableTerminalFailure makes the database task/run state authoritative
// when Asynq is about to stop retrying. Without this boundary Asynq archives
// the queue item while the run remains processing forever.
func withDurableTerminalFailure(
	svc *graphflow.Service,
	handler func(context.Context, *asynq.Task) error,
) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		err := handler(ctx, task)
		if err == nil || svc == nil || svc.TaskRepo == nil || !isTerminalQueueAttempt(ctx, err) {
			return err
		}

		taskID, parseErr := graphFlowTaskID(task)
		if parseErr != nil {
			logger.ErrorContext(ctx, "failed to persist terminal graph task failure", parseErr)
			return err
		}
		failureContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		message := err.Error()
		if len(message) > 4000 {
			message = message[:4000]
		}
		if updateErr := svc.TaskRepo.UpdateTaskFailed(failureContext, taskID, message); updateErr != nil {
			logger.ErrorContext(failureContext, "failed to mark exhausted graph task as failed", updateErr)
		}
		return err
	}
}

func isTerminalQueueAttempt(ctx context.Context, err error) bool {
	if errors.Is(err, asynq.SkipRetry) {
		return true
	}
	retryCount, retryOK := asynq.GetRetryCount(ctx)
	maxRetry, maxOK := asynq.GetMaxRetry(ctx)
	return retryOK && maxOK && retryCount >= maxRetry
}

func graphFlowTaskID(task *asynq.Task) (uuid.UUID, error) {
	if task == nil {
		return uuid.Nil, fmt.Errorf("graph task is nil")
	}
	var payload struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return uuid.Nil, fmt.Errorf("decode graph task payload: %w", err)
	}
	taskID, err := uuid.Parse(payload.TaskID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse graph task id: %w", err)
	}
	return taskID, nil
}
