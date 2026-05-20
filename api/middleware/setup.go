package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/zgiai/ginext/config"
	system_repo "github.com/zgiai/ginext/internal/modules/system/repository"
	"github.com/zgiai/ginext/pkg/database"
	"github.com/zgiai/ginext/pkg/response"
)

func SetupRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GlobalConfig
		if cfg == nil {
			response.Fail(c, response.ErrConfigError)
			c.Abort()
			return
		}

		if isEditionSelfHosted(cfg) {
			setupRepo := system_repo.NewSetupRepository(database.GetDB())
			setupStatus, err := setupRepo.GetSetupStatus()
			if err != nil {
				response.Fail(c, response.ErrDatabaseError)
				c.Abort()
				return
			}
			if setupStatus == nil {
				response.Fail(c, response.ErrSystemNotSetup)
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

func isEditionSelfHosted(cfg *config.Config) bool {
	return cfg.Platform.Edition == "SELF_HOSTED"
}
