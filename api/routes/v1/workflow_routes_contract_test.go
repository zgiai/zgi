package v1

import (
	"os"
	"strings"
	"testing"
)

func TestWorkflowRoutes_WebAppMigrateUserKeepsScopedAndLegacyRoutesBehindWebAppAuth(t *testing.T) {
	source, err := os.ReadFile("workflow_routes.go")
	if err != nil {
		t.Fatalf("read workflow_routes.go: %v", err)
	}

	text := string(source)
	authMiddleware := `protectedWorkflows.Use(middleware.WebAppAuthMiddleware())`
	scopedRoute := `protectedWorkflows.POST("/:web_app_id/migrate-user", handler.MigrateUserForWebApp)`
	legacyRoute := `protectedWorkflows.POST("/migrate-user", handler.MigrateUser)`

	authIndex := strings.Index(text, authMiddleware)
	if authIndex < 0 {
		t.Fatalf("webapp auth middleware missing; want %q", authMiddleware)
	}

	scopedRouteIndex := strings.Index(text, scopedRoute)
	if scopedRouteIndex < 0 {
		t.Fatalf("resource-scoped migrate-user route missing; want %q", scopedRoute)
	}
	if scopedRouteIndex < authIndex {
		t.Fatalf("resource-scoped migrate-user is registered before auth middleware; route index = %d, middleware index = %d", scopedRouteIndex, authIndex)
	}

	legacyRouteIndex := strings.Index(text, legacyRoute)
	if legacyRouteIndex < 0 {
		t.Fatalf("legacy migrate-user route missing; want %q", legacyRoute)
	}
	if legacyRouteIndex < authIndex {
		t.Fatalf("legacy migrate-user is registered before auth middleware; route index = %d, middleware index = %d", legacyRouteIndex, authIndex)
	}
}
