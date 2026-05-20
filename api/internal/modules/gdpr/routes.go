package gdpr

import (
	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/middleware"
)

// RegisterRoutes registers GDPR routes.
// User routes: /console/api/gdpr/* (requires JWT auth)
func (m *Module) RegisterRoutes(v1Group *gin.RouterGroup, accountService interfaces.AccountService) {
	// User GDPR routes (authenticated users)
	gdprGroup := v1Group.Group("/gdpr")
	gdprGroup.Use(middleware.JWTWithOrganizationAndService(accountService))
	{
		// Data export - users can export their own data
		gdprGroup.POST("/export", m.Handler.ExportMyData)

		// Data erasure - users can request deletion of their own data
		gdprGroup.POST("/erase", m.Handler.EraseMyData)

		// Consent management
		gdprGroup.GET("/consents", m.Handler.GetMyConsents)
		gdprGroup.PUT("/consents", m.Handler.UpdateMyConsent)
	}
}
