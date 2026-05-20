package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/container"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	workspaceHandler "github.com/zgiai/ginext/internal/modules/workspace/handler"
	workspaceRepo "github.com/zgiai/ginext/internal/modules/workspace/repository"
	workspaceService "github.com/zgiai/ginext/internal/modules/workspace/service"
	"gorm.io/gorm"
)

func registerWorkspaceRoutesLegacy(
	router *gin.RouterGroup,
	db *gorm.DB,
	accountService interfaces.AccountService,
	serviceContainer *container.ServiceContainer,
) {
	workspaceRepository := workspaceRepo.NewWorkspaceRepository(db)

	workspaceServiceImpl := workspaceService.NewWorkspaceService(workspaceRepository)
	workspaceHandlerObj := workspaceHandler.NewWorkspaceHandler(
		workspaceServiceImpl,
		accountService,
		serviceContainer.GetOrganizationService(),
	)

	workspaceHandlerObj.RegisterRoutes(router)
}

func RegisterWorkspaceRoutes(router *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, consoleWebURL string, serviceContainer *container.ServiceContainer) {
	registerWorkspaceRoutesLegacy(router, db, accountService, serviceContainer)

	// Register tenant permission filter routes
	registerTenantPermissionFilterRoutes(router, serviceContainer)

	// Register department routes
	registerDepartmentRoutes(router, db, accountService, consoleWebURL, serviceContainer)

	// registerTenantAndMemberRoutesLegacy(router, db, accountService, consoleWebURL)
}

// registerTenantPermissionFilterRoutes registers routes for tenant permission filtering
func registerTenantPermissionFilterRoutes(router *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	workspacePermissionFilterService := serviceContainer.GetWorkspacePermissionFilterService()
	workspacePermissionFilterHandler := workspaceHandler.NewWorkspacePermissionFilterHandler(workspacePermissionFilterService)

	workspacePermissionFilterHandler.RegisterRoutes(router)
}

// registerDepartmentRoutes registers routes for department management
func registerDepartmentRoutes(router *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, consoleWebURL string, serviceContainer *container.ServiceContainer) {
	deptService := serviceContainer.GetDepartmentService()
	enterpriseService := serviceContainer.GetOrganizationService()
	deptHandler := workspaceHandler.NewDepartmentHandler(deptService, accountService, enterpriseService, consoleWebURL)

	deptHandler.RegisterRoutes(router)
}

// func registerTenantAndMemberRoutesLegacy(...) { ... }

func registerTenantAndMemberRoutesLegacy(router *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, serviceContainer *container.ServiceContainer, consoleWebURL string) {
	tenantRepo := workspaceRepo.NewWorkspaceRepository(db)
	workspaceMemberRepo := workspaceRepo.NewWorkspaceMemberRepository(db)

	tenantService := workspaceService.NewWorkspaceManagementService(
		db,
		tenantRepo,
		workspaceMemberRepo,
		accountService,
		serviceContainer.GetQuotaService(),
		serviceContainer.GetOrganizationService(),
	)

	enterpriseService := serviceContainer.GetOrganizationService()
	membersHandler := workspaceHandler.NewMembersHandler(tenantService, accountService, enterpriseService, consoleWebURL)

	membersHandler.RegisterRoutes(router)

	// tenant API legacy routes have been removed
}
