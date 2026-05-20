package v1

import (
	"context"

	"github.com/hibiken/asynq"
	workflowHandlerPkg "github.com/zgiai/ginext/internal/modules/app/workflow"
	approvalruntime "github.com/zgiai/ginext/internal/modules/app/workflow/approval"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/queue"
	pkgscheduler "github.com/zgiai/ginext/pkg/scheduler"
)

type approvalTaskRegistry interface {
	Register(taskType string, handler func(context.Context, *asynq.Task) error) bool
}

func registerApprovalTaskHandlers(registry approvalTaskRegistry, taskManager *queue.TaskManager, service *approvalruntime.Service, handler *workflowHandlerPkg.WorkflowHandler) {
	if registry == nil || service == nil || handler == nil {
		return
	}
	taskType := approvalruntime.TypeApprovalResume
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	taskHandler := approvalruntime.NewResumeTaskHandler(service, handler.ResumeApprovalWorkflow)
	if isNew := registry.Register(taskType, taskHandler); isNew {
		logger.Info("Registered approval resume handler", map[string]interface{}{
			"task_type": taskType,
		})
	} else {
		logger.Warn("Approval resume handler was replaced", map[string]interface{}{
			"task_type": taskType,
		})
	}
}

func registerApprovalScheduledTasks(scheduler *pkgscheduler.Scheduler, service *approvalruntime.Service, handler *workflowHandlerPkg.WorkflowHandler) {
	if scheduler == nil || service == nil || handler == nil {
		return
	}
	task := approvalruntime.NewTimeoutScanTask(0)
	taskHandler := approvalruntime.NewTimeoutScanHandler(service, handler.ResumeApprovalWorkflow, 100)
	if err := scheduler.RegisterTask(task, taskHandler); err != nil {
		logger.Error("Failed to register approval timeout scan task", err)
		return
	}
	logger.Info("Approval timeout scan task registered", map[string]interface{}{
		"interval_seconds": int(task.Interval().Seconds()),
	})
}
