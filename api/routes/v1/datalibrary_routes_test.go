package v1

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/container"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRegisterDataLibraryRoutesMountsReadOnlyAssetRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:data-library-routes?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	router := gin.New()
	serviceContainer := container.NewServiceContainer(db, nil, &config.Config{}, nil, nil, nil)
	RegisterDataLibraryRoutes(router.Group("/console/api"), serviceContainer)

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	expected := []string{
		"GET /console/api/data-library/assets",
		"POST /console/api/data-library/assets/sync-file",
		"POST /console/api/data-library/assets/sync-files",
		"GET /console/api/data-library/assets/:asset_id",
		"POST /console/api/data-library/assets/:asset_id/processing-plan",
		"POST /console/api/data-library/assets/:asset_id/processing-requests",
		"GET /console/api/data-library/assets/:asset_id/processing-requests",
		"GET /console/api/data-library/assets/:asset_id/extraction-artifacts",
		"GET /console/api/data-library/assets/:asset_id/vector-artifacts",
		"GET /console/api/data-library/assets/:asset_id/reuse-events",
		"GET /console/api/data-library/extraction-artifacts",
		"GET /console/api/data-library/extraction-artifacts/:artifact_id",
		"GET /console/api/data-library/vector-artifacts",
		"GET /console/api/data-library/vector-artifacts/:artifact_id",
		"GET /console/api/data-library/processing-executors",
		"POST /console/api/data-library/processing-executors/:executor_key/enqueue",
		"POST /console/api/data-library/processing-executors/:executor_key/claim",
		"GET /console/api/data-library/processing-requests",
		"GET /console/api/data-library/processing-requests/summary",
		"POST /console/api/data-library/processing-requests/claim",
		"POST /console/api/data-library/processing-requests/:request_id/queue",
		"POST /console/api/data-library/processing-requests/:request_id/start",
		"POST /console/api/data-library/processing-requests/:request_id/complete",
		"POST /console/api/data-library/processing-requests/:request_id/fail",
		"POST /console/api/data-library/processing-requests/:request_id/cancel",
		"POST /console/api/data-library/processing-requests/:request_id/retry",
	}
	for _, route := range expected {
		if !routes[route] {
			t.Fatalf("expected route %s to be registered; got %#v", route, routes)
		}
	}
}
