package memory

import (
	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
)

func (m *Module) RegisterRoutes(router *gin.RouterGroup, accountService interfaces.AccountService) {
	group := router.Group("/memory")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(accountService))
	{
		group.GET("/me", m.Handler.GetMe)
		group.PATCH("/me/settings", m.Handler.UpdateSettings)
		group.POST("/me/entries", m.Handler.CreateEntry)
		group.PATCH("/me/entries/:entry_id", m.Handler.UpdateEntry)
		group.DELETE("/me/entries/:entry_id", m.Handler.DeleteEntry)
	}
}
