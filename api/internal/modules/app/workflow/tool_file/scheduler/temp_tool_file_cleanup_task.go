package scheduler

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/ginext/pkg/logger"
)

const TaskTypeTempToolFileCleanup = "workflow:tool_file:temp:cleanup"

type TempToolFileCleanupTask struct {
	cronSpec string
}

func NewTempToolFileCleanupTask(cronSpec string) *TempToolFileCleanupTask {
	if cronSpec == "" {
		cronSpec = "0 3 * * *"
	}

	return &TempToolFileCleanupTask{cronSpec: cronSpec}
}

func (t *TempToolFileCleanupTask) TaskType() string {
	return TaskTypeTempToolFileCleanup
}

func (t *TempToolFileCleanupTask) CronSpec() string {
	return t.cronSpec
}

func (t *TempToolFileCleanupTask) Interval() time.Duration {
	return 0
}

func (t *TempToolFileCleanupTask) Payload() []byte {
	return nil
}

func (t *TempToolFileCleanupTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.Queue("scheduler"),
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Minute),
		asynq.Unique(23 * time.Hour),
	}
}

type TempToolFileCleanupHandler struct {
	manager *tool_file.ToolFileManager
}

func NewTempToolFileCleanupHandler(manager *tool_file.ToolFileManager) *TempToolFileCleanupHandler {
	return &TempToolFileCleanupHandler{manager: manager}
}

func (h *TempToolFileCleanupHandler) Handle(ctx context.Context, task *asynq.Task) error {
	logger.Info("Starting temporary tool file cleanup task", nil)

	deleted, err := h.manager.CleanupExpiredTemporaryFiles(ctx)
	if err != nil {
		logger.Error("Temporary tool file cleanup task failed", err)
		return err
	}

	logger.Info("Temporary tool file cleanup task completed", map[string]any{
		"deleted_count": deleted,
	})

	return nil
}
