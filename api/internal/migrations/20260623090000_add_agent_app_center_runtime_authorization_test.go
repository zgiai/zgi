package migrations

import (
	"strings"
	"testing"
)

func TestAddAgentAppCenterRuntimeAuthorizationMigrationSeedsWorkspaceScopedSurface(t *testing.T) {
	sql := compactSQL(allowAppCenterPublishedRuntimeSurfacesSQL + seedAgentAppCenterRuntimeSurfacesSQL + seedAgentAppCenterRuntimeGrantsSQL)
	for _, want := range []string{
		"ADD CONSTRAINT published_runtime_surfaces_surface_check CHECK (surface IN ('webapp', 'api', 'app_center', 'builtin_app', 'internal'))",
		"FROM public.agents",
		"LEFT JOIN public.workspaces ON workspaces.id = agents.tenant_id",
		"'app_center', true, 'legacy_agent_fields'",
		"jsonb_build_object('seeded_from', 'agents_app_center')",
		"WHERE agents.deleted_at IS NULL ON CONFLICT DO NOTHING",
		"INSERT INTO public.published_runtime_surface_grants",
		"'workspace', surfaces.workspace_id, surfaces.enabled",
		"WHERE surfaces.resource_type = 'agent' AND surfaces.surface = 'app_center'",
		"AND surfaces.workspace_id IS NOT NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("add agent app center migration SQL missing %q: %s", want, sql)
		}
	}
	if strings.Contains(sql, "resource_type = 'builtin_workflow'") {
		t.Fatalf("app center migration must not target builtin workflow rows: %s", sql)
	}
}
