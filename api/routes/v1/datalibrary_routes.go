package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/middleware"
)

func RegisterDataLibraryRoutes(v1 *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	group := v1.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(serviceContainer.GetAccountServiceAdapter()))

	serviceContainer.GetDataLibraryModule().DocumentAssetHandler.RegisterRoutes(group)
	serviceContainer.GetDataLibraryModule().VectorArtifactHandler.RegisterRoutes(group)
	serviceContainer.GetDataLibraryModule().ExtractionArtifactHandler.RegisterRoutes(group)
	serviceContainer.GetDataLibraryModule().ProcessingExecutorHandler.RegisterRoutes(group)
}
