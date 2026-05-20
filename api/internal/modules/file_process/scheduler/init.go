package scheduler

import (
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/logger"
	pkgScheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

func RegisterFileTasks(s *pkgScheduler.Scheduler, fileService interfaces.FileService) error {
	if s == nil || fileService == nil {
		return nil
	}

	cronSpec := "0 3 * * *"
	ttlHours := 24

	task := NewTempFileCleanupTask(cronSpec)
	handler := NewTempFileCleanupHandler(fileService, ttlHours)

	if err := s.RegisterTask(task, handler); err != nil {
		return err
	}

	logger.Info("Temporary file cleanup task registered", map[string]interface{}{
		"cron_spec": cronSpec,
		"ttl_hours": ttlHours,
	})

	return nil
}
