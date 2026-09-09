package external

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// RegisterDigitalHumanExternalRoutes keeps DigitalHuman's independently
// authenticated routes outside the legacy Agent API-key middleware group.
func RegisterDigitalHumanExternalRoutes(
	router *gin.RouterGroup,
	runtime *digitalhuman.RuntimeService,
	access *digitalhuman.AccessService,
	fileService interfaces.FileService,
	catalog *appcatalog.Catalog,
) {
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		panic("digital human external routes require application error projector")
	}
	digitalhuman.NewExternalHandler(runtime, access, fileService, projector).RegisterAPIRoutes(router)
}
