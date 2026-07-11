package v1

import (
	"os"
	"strings"
	"testing"
)

func TestAgentsRoutes_WebAppCapabilityStaysBehindWebAppAuthMiddleware(t *testing.T) {
	source, err := os.ReadFile("agents_routers.go")
	if err != nil {
		t.Fatalf("read agents_routers.go: %v", err)
	}

	text := string(source)
	publicConfigRoute := `publicWebApps.GET("/:web_app_id/config", appHandler.GetWebAppRuntimeConfig)`
	publicCapabilityRoute := `publicWebApps.GET("/:web_app_id/capability"`
	authMiddleware := `protectedWebApps.Use(middleware.WebAppAuthMiddleware())`
	protectedCapabilityRoute := `protectedWebApps.GET("/:web_app_id/capability", appHandler.GetWebAppRuntimeCapability)`

	if !strings.Contains(text, publicConfigRoute) {
		t.Fatalf("public webapp config route missing; want %q", publicConfigRoute)
	}
	if strings.Contains(text, publicCapabilityRoute) {
		t.Fatalf("capability route is registered on public webapps group; route = %q", publicCapabilityRoute)
	}

	authIndex := strings.Index(text, authMiddleware)
	if authIndex < 0 {
		t.Fatalf("webapp auth middleware missing; want %q", authMiddleware)
	}

	capabilityIndex := strings.Index(text, protectedCapabilityRoute)
	if capabilityIndex < 0 {
		t.Fatalf("protected webapp capability route missing; want %q", protectedCapabilityRoute)
	}
	if capabilityIndex < authIndex {
		t.Fatalf("protected webapp capability route is registered before auth middleware; route index = %d, middleware index = %d", capabilityIndex, authIndex)
	}
}
