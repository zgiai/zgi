package v1

import (
	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspaceHandler "github.com/zgiai/zgi/api/internal/modules/workspace/handler"
	workspaceRepo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	workspaceService "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"gorm.io/gorm"
)

// WorkspaceRouteDeps contains dependencies required by workspace routes.
type WorkspaceRouteDeps struct {
	DB                               *gorm.DB
	AccountService                   interfaces.AccountService
	OrganizationService              interfaces.OrganizationService
	WorkspacePermissionFilterService workspaceService.WorkspacePermissionFilterService
	DepartmentService                workspaceService.DepartmentService
	ConsoleWebURL                    string
}

func registerWorkspaceRoutesLegacy(
	router *gin.RouterGroup,
	deps WorkspaceRouteDeps,
) {
	workspaceRepository := workspaceRepo.NewWorkspaceRepository(deps.DB)

	workspaceServiceImpl := workspaceService.NewWorkspaceService(workspaceRepository)
	workspaceHandlerObj := workspaceHandler.NewWorkspaceHandler(
		workspaceServiceImpl,
		deps.AccountService,
		deps.OrganizationService,
	)

	workspaceHandlerObj.RegisterRoutes(router)
}

func RegisterWorkspaceRoutes(router *gin.RouterGroup, deps WorkspaceRouteDeps) {
	if deps.DB == nil {
		panic("workspace routes require db")
	}
	if deps.AccountService == nil {
		panic("workspace routes require account service")
	}
	if deps.OrganizationService == nil {
		panic("workspace routes require organization service")
	}
	if deps.WorkspacePermissionFilterService == nil {
		panic("workspace routes require workspace permission filter service")
	}
	if deps.DepartmentService == nil {
		panic("workspace routes require department service")
	}

	registerWorkspaceRoutesLegacy(router, deps)

	// Register tenant permission filter routes
	registerTenantPermissionFilterRoutes(router, deps)

	// Register department routes
	registerDepartmentRoutes(router, deps)

	// Register organization workspace asset move routes
	registerWorkspaceAssetMoveRoutes(router, deps)
}

// registerTenantPermissionFilterRoutes registers routes for tenant permission filtering
func registerTenantPermissionFilterRoutes(router *gin.RouterGroup, deps WorkspaceRouteDeps) {
	workspacePermissionFilterHandler := workspaceHandler.NewWorkspacePermissionFilterHandler(deps.WorkspacePermissionFilterService)

	workspacePermissionFilterHandler.RegisterRoutes(router)
}

// registerDepartmentRoutes registers routes for department management
func registerDepartmentRoutes(router *gin.RouterGroup, deps WorkspaceRouteDeps) {
	deptHandler := workspaceHandler.NewDepartmentHandler(
		deps.DepartmentService,
		deps.AccountService,
		deps.OrganizationService,
		deps.ConsoleWebURL,
	)

	deptHandler.RegisterRoutes(router)
}

func registerWorkspaceAssetMoveRoutes(router *gin.RouterGroup, deps WorkspaceRouteDeps) {
	assetMoveService := workspaceService.NewWorkspaceAssetMoveService(
		deps.DB,
		deps.OrganizationService,
	)
	assetMoveHandler := workspaceHandler.NewWorkspaceAssetMoveHandler(deps.AccountService, assetMoveService)

	assetMoveHandler.RegisterRoutes(router)
}
