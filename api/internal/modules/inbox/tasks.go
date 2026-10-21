package inbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// DeliveryTaskType is the background task that sends one delivery.
const DeliveryTaskType = "inbox:notification:deliver"

// TaskRegistry registers background task handlers.
type TaskRegistry interface {
	Register(string, func(context.Context, *asynq.Task) error) bool
}

// TaskQueue enqueues background tasks.
type TaskQueue interface {
	EnqueueTask(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	GetTaskTypeWithPrefix(string) string
}

// RegisterDeliveryTasks registers the delivery handler and returns the
// enqueuer the dispatcher uses. Each delivery is one task; asynq retries a
// failed attempt with backoff until the dispatcher marks it failed.
func RegisterDeliveryTasks(registry TaskRegistry, queue TaskQueue, dispatcher func() *Dispatcher) DeliveryEnqueuer {
	if registry == nil || queue == nil || dispatcher == nil {
		panic("notification delivery tasks require registry, queue, and dispatcher")
	}
	taskType := queue.GetTaskTypeWithPrefix(DeliveryTaskType)
	if !registry.Register(taskType, func(ctx context.Context, task *asynq.Task) error {
		id, err := uuid.ParseBytes(task.Payload())
		if err != nil {
			return fmt.Errorf("%v: %w", err, asynq.SkipRetry)
		}
		return dispatcher().Deliver(ctx, id)
	}) {
		panic("notification delivery task handler is already registered")
	}
	return func(_ context.Context, deliveryID uuid.UUID) error {
		task := asynq.NewTask(taskType, []byte(deliveryID.String()),
			asynq.Queue("default"), asynq.MaxRetry(MaxDeliveryAttempts-1), asynq.Timeout(time.Minute), asynq.TaskID("inbox-delivery-"+deliveryID.String()))
		if _, err := queue.EnqueueTask(task); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) && !errors.Is(err, asynq.ErrDuplicateTask) {
			return fmt.Errorf("enqueue notification delivery: %w", err)
		}
		return nil
	}
}
