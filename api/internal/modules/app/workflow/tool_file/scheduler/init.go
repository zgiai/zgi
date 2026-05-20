package scheduler

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/pkg/logger"
	pkgScheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

func RegisterToolFileTasks(s *pkgScheduler.Scheduler, manager *tool_file.ToolFileManager) error {
	if s == nil || manager == nil {
		return nil
	}

	cronSpec := "0 3 * * *"
	task := NewTempToolFileCleanupTask(cronSpec)
	handler := NewTempToolFileCleanupHandler(manager)

	if err := s.RegisterTask(task, handler); err != nil {
		return err
	}

	logger.Info("Temporary tool file cleanup task registered", map[string]any{
		"cron_spec": cronSpec,
	})

	return nil
}
