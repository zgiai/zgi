package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/zgimarket"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
)

// RegisterZGIMarketRoutes registers the market routes of an organization's members.
// Path: /workspaces/current/zgi-market/*
// A request is forwarded to the console's market proxy as the caller's current organization.
func RegisterZGIMarketRoutes(
	router *gin.RouterGroup,
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
) {
	settings := zgimarket.Config{}
	if config.GlobalConfig != nil {
		settings.ConsoleURL = config.GlobalConfig.Console.APIURL
		settings.InternalKey = config.GlobalConfig.Console.InternalAPIKey
	}
	proxy := zgimarket.NewProxy(
		settings,
		func(c *gin.Context) string { return util.GetOrganizationID(c) },
		func(c *gin.Context) string { return c.GetString("account_id") },
		func(c *gin.Context, organizationID, accountID string) (bool, bool, error) {
			member, err := accountService.IsOrganizationMember(c.Request.Context(), organizationID, accountID)
			if err != nil || !member {
				return false, false, err
			}
			admin, err := accountService.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, accountID)
			return true, admin, err
		},
	)
	market := router.Group("/workspaces/current/zgi-market",
		middleware.SetAccountService(accountService),
		middleware.SetWorkspaceManagementService(tenantService),
		middleware.JWTWithTenant(),
		setOrganizationContext(accountService),
	)
	market.Any("/*path", proxy.Forward)
}
