package fxapp

import (
	"github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/infra/platform"
	workflowtest "github.com/zgiai/zgi/api/internal/modules/app/workflowtest"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
	workspacerepo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/email"
	"github.com/zgiai/zgi/api/pkg/jwt"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type legacyGlobalsReady struct{}

var legacyGlobalsModule = fx.Module("legacyglobals",
	fx.Provide(
		provideLegacyGlobals,
	),
)

var legacyContainerModule = fx.Module("legacycontainer",
	fx.Provide(
		provideServiceContainer,
		provideBootstrapService,
	),
)

var taskRuntimeModule = fx.Module("taskruntime",
	fx.Provide(
		provideTaskManager,
		provideTaskHandlerRegistry,
		provideWorkflowTestService,
		provideLLMClient,
	),
)

var schedulerModule = fx.Module("scheduler",
	fx.Provide(
		provideScheduler,
	),
)

var graphFlowModule = fx.Module("graphflow",
	fx.Provide(
		provideGraphFlowService,
	),
)

func provideLegacyGlobals(cfg *config.Config) legacyGlobalsReady {
	// These package-level initializers still back legacy code paths that are
	// not constructor-injected yet.
	jwt.Init(cfg)
	email.Init(cfg)
	return legacyGlobalsReady{}
}

// provideServiceContainer bridges the legacy service container into the Fx graph.
// The Redis dependency is intentionally requested to force infra initialization
// to complete before the legacy boot side effects run.
func provideServiceContainer(
	db *gorm.DB,
	cfg *config.Config,
	_ *redis.Client,
	_ legacyGlobalsReady,
	platformContainer *platform.Container,
) (*container.ServiceContainer, error) {
	tokenManager := util.NewTokenManager()

	workspaceRepo := workspacerepo.NewWorkspaceRepository(db)
	workspaceMemberRepo := workspacerepo.NewWorkspaceMemberRepository(db)

	serviceContainer := container.NewServiceContainer(
		db,
		tokenManager,
		cfg,
		workspaceRepo,
		workspaceMemberRepo,
		platformContainer,
	)
	serviceContainer.InitializeDependencies()

	return serviceContainer, nil
}

// Expose concrete runtime dependencies instead of making downstream code pull
// everything from the legacy ServiceContainer directly.
func provideTaskManager(serviceContainer *container.ServiceContainer) *queue.TaskManager {
	return serviceContainer.GetTaskManager()
}

func provideTaskHandlerRegistry(serviceContainer *container.ServiceContainer) *container.TaskHandlerRegistrar {
	return serviceContainer.GetTaskHandlerRegistry()
}

func provideWorkflowTestService(serviceContainer *container.ServiceContainer) *workflowtest.Service {
	return serviceContainer.GetWorkflowTestService()
}

func provideLLMClient(serviceContainer *container.ServiceContainer) llmclient.LLMClient {
	return serviceContainer.GetLLMClient()
}

func provideScheduler(serviceContainer *container.ServiceContainer) *pkgscheduler.Scheduler {
	return serviceContainer.GetScheduler()
}

func provideGraphFlowService(serviceContainer *container.ServiceContainer) *graphflow.Service {
	return serviceContainer.GetGraphFlowService()
}

func provideBootstrapService(serviceContainer *container.ServiceContainer) *system_service.BootstrapService {
	return serviceContainer.GetBootstrapService()
}
