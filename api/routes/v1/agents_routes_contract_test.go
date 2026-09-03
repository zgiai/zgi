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

func TestAgentsRoutes_RollbackPreviewUsesVersionScopedGET(t *testing.T) {
	source, err := os.ReadFile("agents_routers.go")
	if err != nil {
		t.Fatalf("read agents_routers.go: %v", err)
	}
	want := `appsGroup.GET("/:agent_id/published-versions/:version_id/rollback-preview", appHandler.PreviewAgentPublishedVersionRollback)`
	if !strings.Contains(string(source), want) {
		t.Fatalf("rollback preview route missing; want %q", want)
	}
}

func TestAgentsRoutes_ResourceCandidatePickersUseAgentScopedGETs(t *testing.T) {
	source, err := os.ReadFile("agents_routers.go")
	if err != nil {
		t.Fatalf("read agents_routers.go: %v", err)
	}

	text := string(source)
	wants := []string{
		`appsGroup.GET("/:agent_id/candidates/skills", appHandler.ListAgentSkillBindingCandidates)`,
		`appsGroup.GET("/:agent_id/candidates/knowledge", appHandler.ListAgentKnowledgeBindingCandidates)`,
		`appsGroup.GET("/:agent_id/candidates/workflows", appHandler.ListAgentWorkflowBindingCandidates)`,
		`appsGroup.GET("/:agent_id/candidates/databases", appHandler.ListAgentDatabaseBindingCandidates)`,
		`appsGroup.GET("/:agent_id/candidates/databases/:data_source_id/tables", appHandler.ListAgentDatabaseTableBindingCandidates)`,
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("agent resource candidate route missing; want %q", want)
		}
	}
}

func TestAgentsRoutes_DeleteImpactPreviewUsesAgentScopedGET(t *testing.T) {
	source, err := os.ReadFile("agents_routers.go")
	if err != nil {
		t.Fatalf("read agents_routers.go: %v", err)
	}
	want := `appsGroup.GET("/:agent_id/delete-impact", appHandler.PreviewAgentDeleteImpact)`
	if !strings.Contains(string(source), want) {
		t.Fatalf("delete impact preview route missing; want %q", want)
	}
}
