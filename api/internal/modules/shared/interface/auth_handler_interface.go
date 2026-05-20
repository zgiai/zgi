package interfaces

import (
	"github.com/gin-gonic/gin"
)

type AccountHandler interface {
	RegisterRoutes(group *gin.RouterGroup)
}

type AuthHandler interface {
	RegisterAuthRoutes(group *gin.RouterGroup)
}

type ForgotPasswordHandler interface {
	RegisterRoutes(group *gin.RouterGroup)
}

type ActivateHandler interface {
	RegisterRoutes(group *gin.RouterGroup)
}

type AuthHandlerFactory interface {
	NewAccountHandler(accountService AccountService, tenantService WorkspaceManagementService) AccountHandler

	NewAuthHandler(accountService AccountService, featureService FeatureService, tokenManager TokenManager) AuthHandler

	NewForgotPasswordHandler(accountService AccountService) ForgotPasswordHandler

	NewActivateHandler(accountService AccountService) ActivateHandler
}
