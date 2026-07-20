package v1

import (
	"context"

	"github.com/hibiken/asynq"
	workflowHandlerPkg "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
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
	questionTaskType := approvalruntime.TypeQuestionResume
	if taskManager != nil {
		questionTaskType = taskManager.GetTaskTypeWithPrefix(questionTaskType)
	}
	questionTaskHandler := approvalruntime.NewQuestionResumeTaskHandler(handler.ResumeQuestionAnswerWorkflow)
	if isNew := registry.Register(questionTaskType, questionTaskHandler); isNew {
		logger.Info("Registered question resume handler", map[string]interface{}{
			"task_type": questionTaskType,
		})
	} else {
		logger.Warn("Question resume handler was replaced", map[string]interface{}{
			"task_type": questionTaskType,
		})
	}
}

func registerApprovalScheduledTasks(scheduler *pkgscheduler.Scheduler, service *approvalruntime.Service, handler *workflowHandlerPkg.WorkflowHandler, taskManager *queue.TaskManager) {
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
	outboxTask := approvalruntime.NewRuntimeOutboxScanTask(0)
	outboxHandler := approvalruntime.NewRuntimeOutboxScanHandler(service, taskManager, 100)
	if err := scheduler.RegisterTask(outboxTask, outboxHandler); err != nil {
		logger.Error("Failed to register workflow runtime outbox scan task", err)
		return
	}
	logger.Info("Workflow runtime outbox scan task registered", map[string]interface{}{
		"interval_seconds": int(outboxTask.Interval().Seconds()),
	})
	leaseTask := approvalruntime.NewExecutionLeaseScanTask(0)
	leaseHandler := approvalruntime.NewExecutionLeaseScanHandler(service, 100)
	if err := scheduler.RegisterTask(leaseTask, leaseHandler); err != nil {
		logger.Error("Failed to register workflow execution lease scan task", err)
		return
	}
	logger.Info("Workflow execution lease scan task registered", map[string]interface{}{
		"interval_seconds": int(leaseTask.Interval().Seconds()),
	})
}
