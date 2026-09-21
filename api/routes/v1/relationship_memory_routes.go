package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"gorm.io/gorm"
)

func RegisterRelationshipMemoryRoutes(router *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService) {
	if db == nil || accountService == nil {
		panic("relationship memory routes require db and account service")
	}
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(accountService))
	relationshipmemory.NewHandler(relationshipmemory.NewService(db)).RegisterRoutes(group)
}
