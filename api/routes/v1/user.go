package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/container"
	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
	authHandler "github.com/zgiai/zgi/api/internal/modules/user/auth/handler"
	workspaceHandler "github.com/zgiai/zgi/api/internal/modules/workspace/handler"
	workspaceRepo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	workspaceService "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	helper "github.com/zgiai/zgi/api/internal/util"
)

// RegisterUserRoutes registers user-related routes
func RegisterUserRoutes(v1 *gin.RouterGroup, serviceContainer *container.ServiceContainer, consoleWebURL string) {
	accountService := serviceContainer.GetAccountService()
	tenantService := serviceContainer.GetTenantService()
	enterpriseService := serviceContainer.GetOrganizationService()
	workspaceRepository := workspaceRepo.NewWorkspaceRepository(serviceContainer.GetDB())
	workspaceServiceImpl := workspaceService.NewWorkspaceService(workspaceRepository)
	departmentService := serviceContainer.GetDepartmentService()

	tokenMgr := helper.NewTokenManager()
	featureService := system_service.NewFeatureService()

	// --- Handlers ---
	accountHandler := authHandler.NewAccountHandler(accountService, tenantService)
	forgotPasswordHandler := authHandler.NewForgotPasswordHandler(accountService)
	authHandlerInstance := authHandler.NewAuthHandler(accountService, featureService, tokenMgr)
	activateHandler := authHandler.NewActivateHandler(accountService)

	membersHandler := workspaceHandler.NewMembersHandler(
		tenantService,
		accountService,
		enterpriseService,
		consoleWebURL,
	)

	enterpriseHandler := workspaceHandler.NewOrganizationHandler(
		enterpriseService,
		tenantService,
		accountService,
		workspaceServiceImpl,
		departmentService,
		consoleWebURL,
	)

	// --- Route Registration ---
	accountHandler.RegisterRoutes(v1)
	forgotPasswordHandler.RegisterRoutes(v1)
	authHandlerInstance.RegisterAuthRoutes(v1)
	activateHandler.RegisterRoutes(v1)
	enterpriseHandler.RegisterRoutes(v1)
	membersHandler.RegisterRoutes(v1)
}
