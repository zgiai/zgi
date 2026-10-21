package consolesync

import (
	"context"
	"sync"

	"github.com/gin-gonic/gin"
)

// global holds the process-wide sync service so the existing health endpoint can report
// the control-plane state without a new service or a new dependency graph (plan P8).
var (
	globalMu      sync.RWMutex
	globalService *Service
)

// SetGlobal publishes the running sync service.
func SetGlobal(service *Service) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalService = service
}

// Global returns the running sync service, or nil outside Cloud mode.
func Global() *Service {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalService
}

// RegisterStatusRoute attaches the sync snapshot to the existing health group.
//
// One line in routes/v1/health.go keeps the merge footprint minimal.
func RegisterStatusRoute(health *gin.RouterGroup) {
	if health == nil {
		return
	}
	health.GET("/console-sync", func(c *gin.Context) {
		service := Global()
		if service == nil {
			c.JSON(200, gin.H{
				"enabled": false,
				"reason":  "console sync is not running (self-hosted mode or disabled)",
			})
			return
		}
		c.JSON(200, service.Status(context.WithoutCancel(c.Request.Context())))
	})
}

// StatusSnapshot is a convenience for other packages that only need the current state.
func StatusSnapshot(ctx context.Context) (Status, bool) {
	service := Global()
	if service == nil {
		return Status{}, false
	}
	return service.Status(ctx), true
}
