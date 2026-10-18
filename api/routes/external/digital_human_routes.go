package external

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
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
	tasks digitalhuman.TaskHandlerService,
	fileService interfaces.FileService,
	catalog *appcatalog.Catalog,
) {
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		panic("digital human external routes require application error projector")
	}
	var sessions *digitalhuman.WebAppSessionSigner
	if current := config.Current(); current != nil {
		sessions, _ = digitalhuman.NewWebAppSessionSigner(current.JWT.Secret)
	}
	handler := digitalhuman.NewExternalHandler(runtime, access, tasks, fileService, projector, sessions)
	handler.RegisterAPIRoutes(router)
	handler.RegisterWebAppRoutes(router)
}
