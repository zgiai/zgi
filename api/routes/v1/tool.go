package v1

import (
	"github.com/gin-gonic/gin"
	pluginrunnerservice "github.com/zgiai/zgi/api/internal/modules/pluginrunner/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	tools_handler "github.com/zgiai/zgi/api/internal/modules/tools/handler"
	"github.com/zgiai/zgi/api/middleware"
)

// ToolRouteDeps contains dependencies required by tool routes.
type ToolRouteDeps struct {
	ToolManager                *tools.ToolManager
	AccountInstallationService pluginrunnerservice.AccountInstallationService
	MemberSubscriptionService  pluginrunnerservice.MemberSubscriptionService
	AccountService             interfaces.AccountService
	WorkspaceManagementService interfaces.WorkspaceManagementService
}

// RegisterToolRoutes registers tool-related routes
func RegisterToolRoutes(r *gin.RouterGroup, deps ToolRouteDeps) {
	if deps.ToolManager == nil {
		panic("tool routes require tool manager")
	}
	if deps.AccountInstallationService == nil {
		panic("tool routes require account installation service")
	}
	if deps.MemberSubscriptionService == nil {
		panic("tool routes require member subscription service")
	}
	if deps.AccountService == nil {
		panic("tool routes require account service")
	}
	if deps.WorkspaceManagementService == nil {
		panic("tool routes require workspace management service")
	}

	builtinToolsHandler := tools_handler.NewBuiltinToolsHandler(
		deps.ToolManager,
		deps.AccountInstallationService,
		deps.MemberSubscriptionService,
	)

	// Create tool route group with authentication middleware
	toolGroup := r.Group("",
		middleware.SetAccountService(deps.AccountService),
		middleware.SetWorkspaceManagementService(deps.WorkspaceManagementService),
		middleware.JWTWithTenant(),
		setOrganizationContext(deps.AccountService),
	)

	// ========== System installed Tools (ToolManager) - Requires Auth ==========
	toolGroup.GET("/tools/builtin", builtinToolsHandler.ListBuiltinProviders)
	toolGroup.GET("/tools/builtin/:provider", builtinToolsHandler.GetBuiltinProvider)
}
