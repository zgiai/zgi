package v1

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	automationhandler "github.com/zgiai/zgi/api/internal/modules/automation/handler"
	automationrepo "github.com/zgiai/zgi/api/internal/modules/automation/repository"
	automationscheduler "github.com/zgiai/zgi/api/internal/modules/automation/scheduler"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	automationnotification "github.com/zgiai/zgi/api/internal/modules/automation/service/notification"
	automationruntime "github.com/zgiai/zgi/api/internal/modules/automation/service/runtime"
	automationworker "github.com/zgiai/zgi/api/internal/modules/automation/worker"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

type AutomationRouteDeps struct {
	DB                               *gorm.DB
	TaskManager                      *queue.TaskManager
	TaskHandlerRegistry              automationworker.TaskHandlerRegistry
	Scheduler                        *pkgscheduler.Scheduler
	NotificationSMSService           notificationsms.Service
	AutomationWorkflowRunnerProvider func() automationaction.AutomationWorkflowRunner
	AccountService                   interfaces.AccountService
	OrganizationService              interfaces.OrganizationService
	WorkspaceManagementService       interfaces.WorkspaceManagementService
	LLMClient                        llmclient.LLMClient
	DefaultModelService              llmdefaultservice.DefaultModelResolver
}

// RegisterAutomationRoutes wires automation MVP routes, workers, and scheduler registration.
func RegisterAutomationRoutes(router *gin.RouterGroup, deps AutomationRouteDeps) automationdefinition.Service {
	validateAutomationRouteDeps(deps)

	taskRepo := automationrepo.NewTaskRepository(deps.DB)
	actionRepo := automationrepo.NewActionRepository(deps.DB)
	runRepo := automationrepo.NewRunRepository(deps.DB)

	definitionService := automationdefinition.NewService(deps.DB, taskRepo, actionRepo, runRepo, deps.TaskManager)
	notificationExecutor := automationaction.NewNotificationExecutor(
		automationnotification.NewEmailSink(),
		automationnotification.NewNotificationSMSSink(deps.NotificationSMSService),
	)
	runWorkflowExecutor := automationaction.NewRunWorkflowExecutorWithProvider(deps.AutomationWorkflowRunnerProvider)
	executor := automationruntime.NewExecutorWithActionExecutors(deps.DB, taskRepo, actionRepo, runRepo, notificationExecutor, runWorkflowExecutor)

	taskHandler := automationhandler.NewTaskHandler(
		definitionService,
		actionRepo,
		runRepo,
		deps.AccountService,
		deps.OrganizationService,
		deps.WorkspaceManagementService,
		deps.LLMClient,
		deps.DefaultModelService,
	)
	taskHandler.RegisterRoutes(router)

	automationworker.RegisterAutomationHandlers(deps.TaskHandlerRegistry, executor, deps.TaskManager)
	logger.Info("Automation execution handlers registered", map[string]interface{}{
		"isolation": "task_queue_env_prefix",
	})

	if automationDispatchEnabled() {
		if err := automationscheduler.RegisterAutomationTasks(deps.Scheduler, definitionService, 200); err != nil {
			logger.Error("Failed to register automation scheduled tasks", err)
		} else {
			logger.Info("Automation scheduled tasks registered", nil)
		}
	} else {
		logger.Info("Automation scheduled dispatch disabled by configuration", nil)
	}

	return definitionService
}

func validateAutomationRouteDeps(deps AutomationRouteDeps) {
	if deps.DB == nil {
		panic("automation routes require db")
	}
	if deps.TaskManager == nil {
		panic("automation routes require task manager")
	}
	if deps.TaskHandlerRegistry == nil {
		panic("automation routes require task handler registry")
	}
	if deps.Scheduler == nil {
		panic("automation routes require scheduler")
	}
	if deps.NotificationSMSService == nil {
		panic("automation routes require notification sms service")
	}
	if deps.AutomationWorkflowRunnerProvider == nil {
		panic("automation routes require automation workflow runner provider")
	}
	if deps.AccountService == nil {
		panic("automation routes require account service")
	}
	if deps.OrganizationService == nil {
		panic("automation routes require organization service")
	}
	if deps.WorkspaceManagementService == nil {
		panic("automation routes require workspace management service")
	}
	if deps.LLMClient == nil {
		panic("automation routes require llm client")
	}
	if deps.DefaultModelService == nil {
		panic("automation routes require default model service")
	}
}

func automationDispatchEnabled() bool {
	return config.Current().Automation.DispatchEnabled
}
