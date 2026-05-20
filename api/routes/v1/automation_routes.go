package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/container"
	automationhandler "github.com/zgiai/ginext/internal/modules/automation/handler"
	automationrepo "github.com/zgiai/ginext/internal/modules/automation/repository"
	automationscheduler "github.com/zgiai/ginext/internal/modules/automation/scheduler"
	automationaction "github.com/zgiai/ginext/internal/modules/automation/service/action"
	automationdefinition "github.com/zgiai/ginext/internal/modules/automation/service/definition"
	automationnotification "github.com/zgiai/ginext/internal/modules/automation/service/notification"
	automationruntime "github.com/zgiai/ginext/internal/modules/automation/service/runtime"
	automationworker "github.com/zgiai/ginext/internal/modules/automation/worker"
	"github.com/zgiai/ginext/pkg/logger"
)

// RegisterAutomationRoutes wires automation MVP routes, workers, and scheduler registration.
func RegisterAutomationRoutes(router *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	db := serviceContainer.GetDB()
	taskManager := serviceContainer.GetTaskManager()

	taskRepo := automationrepo.NewTaskRepository(db)
	actionRepo := automationrepo.NewActionRepository(db)
	runRepo := automationrepo.NewRunRepository(db)

	definitionService := automationdefinition.NewService(db, taskRepo, actionRepo, runRepo, taskManager)
	serviceContainer.SetAutomationDefinitionService(definitionService)
	notificationExecutor := automationaction.NewNotificationExecutor(
		automationnotification.NewEmailSink(),
		automationnotification.NewNotificationSMSSink(serviceContainer.GetNotificationSMSService()),
	)
	runWorkflowExecutor := automationaction.NewRunWorkflowExecutorWithProvider(serviceContainer.GetAutomationWorkflowRunner)
	executor := automationruntime.NewExecutorWithActionExecutors(db, taskRepo, actionRepo, runRepo, notificationExecutor, runWorkflowExecutor)

	taskHandler := automationhandler.NewTaskHandler(
		definitionService,
		actionRepo,
		runRepo,
		serviceContainer.GetAccountServiceAdapter(),
		serviceContainer.GetOrganizationService(),
		serviceContainer.GetTenantServiceAdapter(),
		serviceContainer.GetLLMClient(),
		serviceContainer.GetDefaultModelService(),
	)
	taskHandler.RegisterRoutes(router)

	automationworker.RegisterAutomationHandlers(serviceContainer.GetTaskHandlerRegistry(), executor, taskManager)
	logger.Info("Automation execution handlers registered", map[string]interface{}{
		"isolation": "task_queue_env_prefix",
	})

	if automationDispatchEnabled() {
		if err := automationscheduler.RegisterAutomationTasks(serviceContainer.GetScheduler(), definitionService, 200); err != nil {
			logger.Error("Failed to register automation scheduled tasks", err)
		} else {
			logger.Info("Automation scheduled tasks registered", nil)
		}
	} else {
		logger.Info("Automation scheduled dispatch disabled by configuration", nil)
	}
}

func automationDispatchEnabled() bool {
	return config.Current().Automation.DispatchEnabled
}
