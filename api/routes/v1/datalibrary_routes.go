package v1

import (
	"github.com/gin-gonic/gin"
	datalibrarymodule "github.com/zgiai/zgi/api/internal/modules/datalibrary"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
)

// DataLibraryRouteDeps contains dependencies required by data library routes.
type DataLibraryRouteDeps struct {
	AccountService    interfaces.AccountService
	DataLibraryModule *datalibrarymodule.Module
}

func RegisterDataLibraryRoutes(v1 *gin.RouterGroup, deps DataLibraryRouteDeps) {
	if deps.AccountService == nil {
		panic("data library routes require account service")
	}
	if deps.DataLibraryModule == nil {
		panic("data library routes require data library module")
	}
	if deps.DataLibraryModule.DocumentAssetHandler == nil {
		panic("data library routes require document asset handler")
	}
	if deps.DataLibraryModule.VectorArtifactHandler == nil {
		panic("data library routes require vector artifact handler")
	}
	if deps.DataLibraryModule.ExtractionArtifactHandler == nil {
		panic("data library routes require extraction artifact handler")
	}
	if deps.DataLibraryModule.ProcessingExecutorHandler == nil {
		panic("data library routes require processing executor handler")
	}

	group := v1.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))

	deps.DataLibraryModule.DocumentAssetHandler.RegisterRoutes(group)
	deps.DataLibraryModule.VectorArtifactHandler.RegisterRoutes(group)
	deps.DataLibraryModule.ExtractionArtifactHandler.RegisterRoutes(group)
	deps.DataLibraryModule.ProcessingExecutorHandler.RegisterRoutes(group)
}
