package queue

import (
	"context"
	"errors"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// asynqHandlerNotFound is the error text Asynq's ServeMux returns for a task
// type it has no handler for. Asynq does not export a sentinel for it.
const asynqHandlerNotFound = "handler not found for task"

// NewTaskFailureHandler reports every failed delivery of a worker with its
// task type and id. A delivery that will be archived (retries exhausted or
// retry skipped) and a delivery no handler exists for are logged as errors:
// Asynq keeps them silently in the archive otherwise, and the domain record
// that waits for them is settled by its own reconciliation.
func NewTaskFailureHandler(worker string) asynq.ErrorHandler {
	return asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
		retried, _ := asynq.GetRetryCount(ctx)
		maxRetry, _ := asynq.GetMaxRetry(ctx)
		exhausted, missingHandler := classifyTaskFailure(retried, maxRetry, err)
		// WithTaskContext adds task_type, task_id, queue and the retry counters.
		taskCtx := logger.WithTaskContext(ctx, task)
		args := []interface{}{"worker", worker, "exhausted", exhausted, "missing_handler", missingHandler, err}
		switch {
		case missingHandler:
			logger.ErrorContext(taskCtx, "task delivery has no handler on this worker", args...)
		case exhausted:
			logger.ErrorContext(taskCtx, "task delivery exhausted its retries and is archived", args...)
		default:
			logger.WarnContext(taskCtx, "task delivery failed and will be retried", args...)
		}
	})
}

// classifyTaskFailure mirrors Asynq's archive decision for a failed delivery.
func classifyTaskFailure(retried, maxRetry int, err error) (exhausted, missingHandler bool) {
	exhausted = retried >= maxRetry || errors.Is(err, asynq.SkipRetry)
	missingHandler = err != nil && strings.Contains(err.Error(), asynqHandlerNotFound)
	return exhausted, missingHandler
}
