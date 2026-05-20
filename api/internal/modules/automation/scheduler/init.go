package scheduler

import (
	automationdefinition "github.com/zgiai/ginext/internal/modules/automation/service/definition"
	pkgscheduler "github.com/zgiai/ginext/pkg/scheduler"
)

// RegisterAutomationTasks registers the fixed due-task dispatcher.
func RegisterAutomationTasks(s *pkgscheduler.Scheduler, service automationdefinition.Service, batchSize int) error {
	if s == nil || service == nil {
		return nil
	}

	dispatchTask := NewDispatchDueTask(batchSize)
	dispatchHandler := NewDispatchDueHandler(service, batchSize)
	return s.RegisterTask(dispatchTask, dispatchHandler)
}
