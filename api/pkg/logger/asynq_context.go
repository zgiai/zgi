package logger

import (
	"context"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

// WithTaskContext attaches asynq task metadata to a context for structured logging.
func WithTaskContext(ctx context.Context, task *asynq.Task) context.Context {
	fields := make([]zap.Field, 0, 5)

	if task != nil && task.Type() != "" {
		fields = append(fields, zap.String("task_type", task.Type()))
	}
	if taskID, ok := asynq.GetTaskID(ctx); ok && taskID != "" {
		fields = append(fields, zap.String("task_id", taskID))
	}
	if queueName, ok := asynq.GetQueueName(ctx); ok && queueName != "" {
		fields = append(fields, zap.String("queue", queueName))
	}
	if retryCount, ok := asynq.GetRetryCount(ctx); ok {
		fields = append(fields, zap.Int("retry_count", retryCount))
	}
	if maxRetry, ok := asynq.GetMaxRetry(ctx); ok {
		fields = append(fields, zap.Int("max_retry", maxRetry))
	}

	if len(fields) == 0 {
		if ctx == nil {
			return context.Background()
		}
		return ctx
	}
	return WithFields(ctx, fields...)
}
