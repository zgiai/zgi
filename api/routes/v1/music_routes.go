package v1

import (
	"github.com/gin-gonic/gin"
	musicmodule "github.com/zgiai/zgi/api/internal/modules/music"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
	"gorm.io/gorm"
)

type MusicRouteDeps struct {
	DB              *gorm.DB
	AvailableModels musicmodule.AvailableModelLister
	Generator       musicmodule.Generator
	LyricsGenerator musicmodule.LyricsGenerator
	Compensator     musicmodule.DeliveryCompensator
	AccountService  interfaces.AccountService
	TaskManager     *queue.TaskManager
	TaskRegistry    musicmodule.TaskRegistry
	Scheduler       *pkgscheduler.Scheduler
}

func RegisterMusicRoutes(router *gin.RouterGroup, deps MusicRouteDeps) {
	if deps.DB == nil || deps.AvailableModels == nil || deps.Generator == nil || deps.LyricsGenerator == nil || deps.Compensator == nil ||
		deps.AccountService == nil || deps.TaskManager == nil || deps.TaskRegistry == nil || deps.Scheduler == nil {
		panic("music routes require database, model catalog, generator, delivery compensator, auth, queue, registry, and scheduler")
	}

	repo := musicmodule.NewRepository(deps.DB)
	dispatcher := musicmodule.NewDispatcher(deps.TaskManager)
	assets := musicmodule.NewToolFileAssetStore()
	service := musicmodule.NewService(repo, dispatcher, deps.AvailableModels, assets)
	worker := musicmodule.NewWorker(repo, dispatcher, deps.Generator, deps.LyricsGenerator, deps.Compensator, assets)
	musicmodule.RegisterTaskHandlers(deps.TaskRegistry, deps.TaskManager, worker)
	if err := deps.Scheduler.RegisterTask(
		&musicmodule.ReconcileTask{},
		musicmodule.NewReconcileHandler(repo, dispatcher),
	); err != nil {
		panic("register music task reconciler: " + err.Error())
	}

	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	musicmodule.NewHandler(service).RegisterRoutes(group)
	logger.Info("Music task routes registered", "path", "/console/api/music/tasks")
}
