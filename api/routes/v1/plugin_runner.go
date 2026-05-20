package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/client"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/handler"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/repository"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/service"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/middleware"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/response"
	"gorm.io/gorm"
)

// RegisterPluginRunnerTenantRoutes registers organization-level Plugin Runner routes
// Path: /workspaces/current/plugin-runner/*
// All organization members can view and subscribe to plugins
// ORG Owner/Admin can register, install, delete plugins
func RegisterPluginRunnerTenantRoutes(
	router *gin.RouterGroup,
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
	db *gorm.DB,
) {
	// Create Plugin Runner service with repositories for database persistence
	cfg := client.NewConfigFromEnv()
	installRepo := repository.NewAccountInstallationRepository(db)
	infoRepo := repository.NewInstalledPluginInfoRepository(db)
	svc := service.NewPluginRunnerServiceWithRepos(cfg, installRepo, infoRepo)
	installSvc := service.NewAccountInstallationService(installRepo, infoRepo)

	// Create Subscription service and handler (reuse repositories)
	subRepo := repository.NewMemberSubscriptionRepository(db)
	subSvc := service.NewMemberSubscriptionService(subRepo, installRepo, infoRepo)
	subHandler := handler.NewSubscriptionHandler(subSvc, accountService)

	h := handler.NewHandler(svc, installSvc, infoRepo, accountService, subSvc)

	// ============================================
	// Organization Routes (All Members)
	// ============================================
	// These routes allow organization members to view and subscribe to plugins
	workspaces := router.Group("/workspaces",
		middleware.SetAccountService(accountService),
		middleware.SetWorkspaceManagementService(tenantService),
		middleware.JWTWithTenant(),
		setOrganizationContext(accountService),
	)
	{
		current := workspaces.Group("/current")
		{
			pluginRunner := current.Group("/plugin-runner")
			{
				// View plugins (all org members)
				pluginRunner.GET("/plugins", h.TenantListPlugins)
				pluginRunner.GET("/plugins/:id", h.TenantGetPlugin)

				// Subscription management (all org members can subscribe/unsubscribe)
				pluginRunner.GET("/plugins/subscribed", subHandler.ListSubscriptions)
				pluginRunner.GET("/plugins/:id/subscribed", subHandler.IsSubscribed)
				pluginRunner.POST("/plugins/:id/subscribe", subHandler.Subscribe)
				pluginRunner.DELETE("/plugins/:id/subscribe", subHandler.Unsubscribe)

				// ============================================
				// Plugin Management (ORG Owner/Admin Only)
				// ============================================
				// These routes allow org owner/admin to register and install plugins
				management := pluginRunner.Group("/management",
					middleware.EnterpriseAdminOrOwnerRequired(),
				)
				{
					management.GET("/plugins", h.ListPlugins)
					management.POST("/plugins", h.RegisterPlugin)
					management.GET("/plugins/installed", h.ListInstalledPlugins)
					management.GET("/plugins/:id", h.GetPlugin)
					management.DELETE("/plugins/:id", h.DeletePlugin)
					management.POST("/plugins/:id/install", h.InstallPlugin)
					management.POST("/plugins/:id/install-base64", h.InstallPluginBase64)
					management.POST("/plugins/install-from-marketplace", h.InstallFromMarketplace)
					management.POST("/plugins/reinstall-from-marketplace", h.ReinstallFromMarketplace)
				}

				// Session management (org owner/admin)
				sessions := pluginRunner.Group("/sessions",
					middleware.EnterpriseAdminOrOwnerRequired(),
				)
				{
					sessions.GET("", h.ListSessions)
					sessions.POST("/:id/stop", h.StopSession)
				}
			}
		}
	}

	logger.Info("plugin runner organization routes registered",
		"path", "/workspaces/current/plugin-runner/*",
	)
}

func setOrganizationContext(accountService interfaces.AccountService) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountID := c.GetString("account_id")
		if accountID == "" {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}

		organizationID, err := accountService.EnsureCurrentOrganizationID(c.Request.Context(), accountID)
		if err != nil || organizationID == "" {
			response.Fail(c, response.ErrOrganizationNotFound)
			c.Abort()
			return
		}

		util.SetOrganizationScopeCompat(c, organizationID)
		c.Next()
	}
}
