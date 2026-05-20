package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/container"
	tools_handler "github.com/zgiai/zgi/api/internal/modules/tools/handler"
	"github.com/zgiai/zgi/api/middleware"
)

// RegisterToolRoutes registers tool-related routes
func RegisterToolRoutes(r *gin.RouterGroup, container *container.ServiceContainer) {
	builtinToolsHandler := tools_handler.NewBuiltinToolsHandler(
		container.GetToolManager(),
		container.GetAccountInstallationService(),
		container.GetMemberSubscriptionService())

	// Create tool route group with authentication middleware
	toolGroup := r.Group("",
		middleware.SetAccountService(container.GetAccountServiceAdapter()),
		middleware.SetWorkspaceManagementService(container.GetTenantServiceAdapter()),
		middleware.JWTWithTenant(),
		setOrganizationContext(container.GetAccountServiceAdapter()),
	)

	// ========== System installed Tools (ToolManager) - Requires Auth ==========
	toolGroup.GET("/tools/builtin", builtinToolsHandler.ListBuiltinProviders)
	toolGroup.GET("/tools/builtin/:provider", builtinToolsHandler.GetBuiltinProvider)
}
