package v1

import (
	"github.com/gin-gonic/gin"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
	authHandler "github.com/zgiai/zgi/api/internal/modules/user/auth/handler"
	workspaceHandler "github.com/zgiai/zgi/api/internal/modules/workspace/handler"
	workspaceRepo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	workspaceService "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	helper "github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
)

// UserRouteDeps contains dependencies required by user and auth routes.
type UserRouteDeps struct {
	DB                         *gorm.DB
	AccountService             interfaces.AccountService
	WorkspaceManagementService interfaces.WorkspaceManagementService
	OrganizationService        interfaces.OrganizationService
	DepartmentService          workspaceService.DepartmentService
	ConsoleWebURL              string
}

// RegisterUserRoutes registers user-related routes
func RegisterUserRoutes(v1 *gin.RouterGroup, deps UserRouteDeps) {
	if deps.DB == nil {
		panic("user routes require db")
	}
	if deps.AccountService == nil {
		panic("user routes require account service")
	}
	if deps.WorkspaceManagementService == nil {
		panic("user routes require workspace management service")
	}
	if deps.OrganizationService == nil {
		panic("user routes require organization service")
	}
	if deps.DepartmentService == nil {
		panic("user routes require department service")
	}

	workspaceRepository := workspaceRepo.NewWorkspaceRepository(deps.DB)
	workspaceServiceImpl := workspaceService.NewWorkspaceService(workspaceRepository)

	tokenMgr := helper.NewTokenManager()
	featureService := system_service.NewFeatureService()

	// --- Handlers ---
	accountHandler := authHandler.NewAccountHandler(deps.AccountService, deps.WorkspaceManagementService)
	forgotPasswordHandler := authHandler.NewForgotPasswordHandler(deps.AccountService)
	authHandlerInstance := authHandler.NewAuthHandler(deps.AccountService, featureService, tokenMgr)
	activateHandler := authHandler.NewActivateHandler(deps.AccountService)

	membersHandler := workspaceHandler.NewMembersHandler(
		deps.WorkspaceManagementService,
		deps.AccountService,
		deps.OrganizationService,
		deps.ConsoleWebURL,
	)

	enterpriseHandler := workspaceHandler.NewOrganizationHandler(
		deps.OrganizationService,
		deps.WorkspaceManagementService,
		deps.AccountService,
		workspaceServiceImpl,
		deps.DepartmentService,
		deps.ConsoleWebURL,
	)

	// --- Route Registration ---
	accountHandler.RegisterRoutes(v1)
	forgotPasswordHandler.RegisterRoutes(v1)
	authHandlerInstance.RegisterAuthRoutes(v1)
	activateHandler.RegisterRoutes(v1)
	enterpriseHandler.RegisterRoutes(v1)
	membersHandler.RegisterRoutes(v1)
}
