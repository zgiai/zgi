package service

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const RuntimeLeaseCleanupTaskType = "system:chat_runtime:lease_cleanup"

type RuntimeLeaseCleanupTask struct{}

func NewRuntimeLeaseCleanupTask() *RuntimeLeaseCleanupTask {
	return &RuntimeLeaseCleanupTask{}
}

func (t *RuntimeLeaseCleanupTask) TaskType() string {
	return RuntimeLeaseCleanupTaskType
}

func (t *RuntimeLeaseCleanupTask) CronSpec() string {
	return ""
}

func (t *RuntimeLeaseCleanupTask) Interval() time.Duration {
	return time.Minute
}

func (t *RuntimeLeaseCleanupTask) Payload() []byte {
	return nil
}

func (t *RuntimeLeaseCleanupTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.MaxRetry(1),
		asynq.Timeout(30 * time.Second),
		asynq.Unique(45 * time.Second),
	}
}

type runtimeLeaseCleanupService interface {
	CleanupStaleActiveMessages(ctx context.Context) (int64, error)
}

type RuntimeLeaseCleanupHandler struct {
	service runtimeLeaseCleanupService
}

func NewRuntimeLeaseCleanupHandler(service runtimeLeaseCleanupService) *RuntimeLeaseCleanupHandler {
	return &RuntimeLeaseCleanupHandler{service: service}
}

func (h *RuntimeLeaseCleanupHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.service == nil {
		return nil
	}
	affected, err := h.service.CleanupStaleActiveMessages(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "chat runtime lease cleanup failed", err)
		return err
	}
	logger.InfoContext(ctx, "chat runtime lease cleanup completed", "affected_count", affected)
	return nil
}
