package v1

import (
	"errors"

	"github.com/gin-gonic/gin"
	shortlinkcap "github.com/zgiai/zgi/api/internal/capabilities/shortlink"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

type ShortLinkRouteDeps struct {
	ShortLinkService shortlinkcap.Service
	Scheduler        *pkgscheduler.Scheduler
}

type shortLinkResolveResponse struct {
	TargetPath string `json:"target_path"`
}

func RegisterShortLinkRoutes(router *gin.RouterGroup, deps ShortLinkRouteDeps) {
	if deps.ShortLinkService == nil {
		panic("short link routes require short link service")
	}
	registerShortLinkScheduledTasks(deps.Scheduler, deps.ShortLinkService)

	group := router.Group("/short-link-resolutions")
	group.Use(middleware.SetupRequired())
	group.GET("/:token", func(c *gin.Context) {
		link, err := deps.ShortLinkService.Resolve(c.Request.Context(), c.Param("token"))
		if err != nil {
			switch {
			case errors.Is(err, shortlinkcap.ErrInvalidToken):
				response.Fail(c, response.ErrInvalidParam)
			case errors.Is(err, shortlinkcap.ErrNotFound), errors.Is(err, shortlinkcap.ErrExpired):
				response.Fail(c, response.ErrNotFound)
			default:
				response.FailWithMessage(c, response.ErrSystemError, err.Error())
			}
			return
		}

		response.Success(c, shortLinkResolveResponse{TargetPath: link.TargetPath})
	})
}

func registerShortLinkScheduledTasks(scheduler *pkgscheduler.Scheduler, service shortlinkcap.Service) {
	if scheduler == nil || service == nil {
		return
	}
	task := shortlinkcap.NewCleanupExpiredTask("")
	taskHandler := shortlinkcap.NewCleanupExpiredHandler(service)
	if err := scheduler.RegisterTask(task, taskHandler); err != nil {
		logger.Error("Failed to register short link cleanup task", err)
		return
	}
	logger.Info("Short link cleanup task registered", map[string]interface{}{
		"cron_spec": task.CronSpec(),
	})
}
