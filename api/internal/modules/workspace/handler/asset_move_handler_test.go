package handler

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWorkspaceAssetMoveHandlerRegistersPreflightRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := NewWorkspaceAssetMoveHandler(nil, nil)
	handler.RegisterRoutes(engine.Group("/console/api"))

	wantPaths := map[string]bool{
		"/console/api/organizations/current/assets/move/eligible-targets": false,
		"/console/api/organizations/current/assets/move/dependencies":     false,
	}
	for _, route := range engine.Routes() {
		if route.Method == "POST" {
			if _, wanted := wantPaths[route.Path]; wanted {
				wantPaths[route.Path] = true
			}
		}
	}
	for path, registered := range wantPaths {
		if !registered {
			t.Fatalf("route POST %s was not registered", path)
		}
	}
}
