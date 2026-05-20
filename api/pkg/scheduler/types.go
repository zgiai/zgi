package scheduler

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// ScheduledTask defines a schedulable task interface
type ScheduledTask interface {
	// TaskType returns the task type identifier
	TaskType() string

	// CronSpec returns the cron expression (if empty, use Interval)
	CronSpec() string

	// Interval returns the execution interval (used when CronSpec is empty)
	Interval() time.Duration

	// Payload returns the task payload data
	Payload() []byte

	// Options returns task options (retry count, timeout, etc.)
	Options() []asynq.Option
}

// TaskHandler defines the task handler interface
type TaskHandler interface {
	// Handle processes the task
	Handle(ctx context.Context, task *asynq.Task) error
}

// SchedulerConfig holds scheduler configuration
type SchedulerConfig struct {
	// Location timezone setting
	Location *time.Location

	// LogLevel log level
	LogLevel asynq.LogLevel

	// PostEnqueueFunc callback after task enqueue
	PostEnqueueFunc func(info *asynq.TaskInfo, err error)
}

// TaskRegistration holds task and handler pair for registration
type TaskRegistration struct {
	Task    ScheduledTask
	Handler TaskHandler
}
