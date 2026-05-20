package scheduler

import (
	"context"
	"time"

	"github.com/hibiken/asynq"

	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/pkg/logger"
)

const (
	TaskTypeTempFileCleanup = "file:temp:cleanup"
)

type TempFileCleanupTask struct {
	cronSpec string
}

func NewTempFileCleanupTask(cronSpec string) *TempFileCleanupTask {
	if cronSpec == "" {
		cronSpec = "0 3 * * *"
	}
	return &TempFileCleanupTask{
		cronSpec: cronSpec,
	}
}

func (t *TempFileCleanupTask) TaskType() string {
	return TaskTypeTempFileCleanup
}

func (t *TempFileCleanupTask) CronSpec() string {
	return t.cronSpec
}

func (t *TempFileCleanupTask) Interval() time.Duration {
	return 0
}

func (t *TempFileCleanupTask) Payload() []byte {
	return nil
}

func (t *TempFileCleanupTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.Queue("scheduler"),
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Minute),
		asynq.Unique(23 * time.Hour),
	}
}

type TempFileCleanupHandler struct {
	fileService interfaces.FileService
	ttlHours    int
}

func NewTempFileCleanupHandler(fileService interfaces.FileService, ttlHours int) *TempFileCleanupHandler {
	if ttlHours <= 0 {
		ttlHours = 24
	}
	return &TempFileCleanupHandler{
		fileService: fileService,
		ttlHours:    ttlHours,
	}
}

func (h *TempFileCleanupHandler) Handle(ctx context.Context, task *asynq.Task) error {
	logger.Info("Starting temporary file cleanup task", map[string]interface{}{
		"ttl_hours": h.ttlHours,
	})

	deleted, err := h.fileService.CleanupExpiredTemporaryFiles(ctx, time.Duration(h.ttlHours)*time.Hour)
	if err != nil {
		logger.Error("Temporary file cleanup task failed", err)
		return err
	}

	logger.Info("Temporary file cleanup task completed", map[string]interface{}{
		"deleted_count": deleted,
	})

	return nil
}
