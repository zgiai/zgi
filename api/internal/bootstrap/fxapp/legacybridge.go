package fxapp

import (
	"github.com/redis/go-redis/v9"
	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/infra/platform"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow"
	workspacerepo "github.com/zgiai/ginext/internal/modules/workspace/repository"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/email"
	"github.com/zgiai/ginext/pkg/jwt"
	"github.com/zgiai/ginext/pkg/queue"
	pkgscheduler "github.com/zgiai/ginext/pkg/scheduler"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// legacyBridgeModule isolates the pre-Fx wiring model behind a narrow bridge.
// It keeps global initializers and ServiceContainer-based assembly in one place,
// then exposes only the concrete runtime dependencies that the Fx app still needs.
var legacyBridgeModule = fx.Module("legacybridge",
	fx.Provide(
		provideServiceContainer,
		provideTaskManager,
		provideTaskHandlerRegistry,
		provideScheduler,
		provideGraphFlowService,
	),
)

// provideServiceContainer bridges the legacy container into the Fx graph.
// The Redis dependency is intentionally requested to force infra initialization
// to complete before the legacy boot side effects run.
func provideServiceContainer(
	db *gorm.DB,
	cfg *config.Config,
	_ *redis.Client,
	platformContainer *platform.Container,
) (*container.ServiceContainer, error) {
	// Global config and DB are initialized by base/infra providers before this bridge runs.
	// These package-level initializers still back legacy code paths that are
	// not constructor-injected yet.
	jwt.Init(cfg)
	email.Init(cfg)

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

func provideScheduler(serviceContainer *container.ServiceContainer) *pkgscheduler.Scheduler {
	return serviceContainer.GetScheduler()
}

func provideGraphFlowService(serviceContainer *container.ServiceContainer) *graphflow.Service {
	return serviceContainer.GetGraphFlowService()
}
