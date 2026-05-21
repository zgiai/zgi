package announcement

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const TypeAnnouncementCleanup = "workflow:announcement:cleanup"

type CleanupExpiredTask struct {
	cronSpec string
}

func NewCleanupExpiredTask(cronSpec string) *CleanupExpiredTask {
	if cronSpec == "" {
		cronSpec = "0 3 * * *"
	}
	return &CleanupExpiredTask{cronSpec: cronSpec}
}

func (t *CleanupExpiredTask) TaskType() string {
	return TypeAnnouncementCleanup
}

func (t *CleanupExpiredTask) CronSpec() string {
	return t.cronSpec
}

func (t *CleanupExpiredTask) Interval() time.Duration {
	return 0
}

func (t *CleanupExpiredTask) Payload() []byte {
	return nil
}

func (t *CleanupExpiredTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.Queue("scheduler"),
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Minute),
		asynq.Unique(23 * time.Hour),
	}
}

type CleanupExpiredHandler struct {
	service *Service
}

func NewCleanupExpiredHandler(service *Service) *CleanupExpiredHandler {
	return &CleanupExpiredHandler{service: service}
}

func (h *CleanupExpiredHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.service == nil {
		return nil
	}
	deleted, err := h.service.CleanupExpiredAnnouncements(ctx, time.Now())
	if err != nil {
		logger.ErrorContext(ctx, "expired announcement cleanup failed", err)
		return err
	}
	logger.InfoContext(ctx, "expired announcement cleanup completed", "deleted_count", deleted)
	return nil
}
