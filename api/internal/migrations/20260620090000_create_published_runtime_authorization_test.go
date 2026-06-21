package migrations

import (
	"strings"
	"testing"
)

func TestPublishedRuntimeAuthorizationMigrationDefinesSurfacesAndGrants(t *testing.T) {
	tableSQL := compactSQL(createPublishedRuntimeSurfacesSQL + createPublishedRuntimeSurfaceGrantsSQL)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS public.published_runtime_surfaces",
		"resource_type varchar(32) NOT NULL",
		"organization_id uuid NOT NULL",
		"workspace_id uuid",
		"surface varchar(32) NOT NULL",
		"CHECK (resource_type IN ('agent', 'builtin_workflow'))",
		"CHECK (surface IN ('webapp', 'api', 'builtin_app', 'internal'))",
		"CREATE TABLE IF NOT EXISTS public.published_runtime_surface_grants",
		"surface_id uuid NOT NULL REFERENCES public.published_runtime_surfaces(id) ON DELETE CASCADE",
		"CHECK (subject_type IN ('public', 'organization', 'department', 'account', 'internal'))",
	} {
		if !strings.Contains(tableSQL, want) {
			t.Fatalf("published runtime authorization table SQL missing %q: %s", want, tableSQL)
		}
	}
}

func TestPublishedRuntimeAuthorizationMigrationDefinesActiveIndexes(t *testing.T) {
	indexSQL := compactSQL(createPublishedRuntimeAuthorizationIndexesSQL)
	for _, want := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_published_runtime_surfaces_active_unique",
		"ON public.published_runtime_surfaces (resource_type, resource_id, surface)",
		"WHERE deleted_at IS NULL",
		"CREATE INDEX IF NOT EXISTS idx_published_runtime_surfaces_org",
		"ON public.published_runtime_surfaces (organization_id, surface)",
		"CREATE INDEX IF NOT EXISTS idx_published_runtime_surface_grants_subject",
		"ON public.published_runtime_surface_grants (subject_type, subject_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_published_runtime_surface_grants_subject_active",
		"WHERE deleted_at IS NULL AND subject_id IS NOT NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_published_runtime_surface_grants_null_subject_active",
		"WHERE deleted_at IS NULL AND subject_id IS NULL",
	} {
		if !strings.Contains(indexSQL, want) {
			t.Fatalf("published runtime authorization index SQL missing %q: %s", want, indexSQL)
		}
	}
}

func TestPublishedRuntimeAuthorizationMigrationSeedsLegacyAgentSemantics(t *testing.T) {
	seedSQL := compactSQL(seedPublishedRuntimeAgentSurfacesSQL + seedPublishedRuntimeAgentSurfaceGrantsSQL)
	for _, want := range []string{
		"FROM public.agents",
		"LEFT JOIN public.workspaces ON workspaces.id = agents.tenant_id",
		"CROSS JOIN (VALUES ('webapp'), ('api'), ('builtin_app'), ('internal')) AS surfaces(surface)",
		"WHEN 'webapp' THEN COALESCE(NULLIF(agents.web_app_status, ''), 'active') = 'active'",
		"WHEN 'api' THEN agents.enable_api",
		"WHEN 'internal' THEN true",
		"ELSE false",
		"WHEN 'internal' THEN 'internal'",
		"ELSE 'public'",
		"surfaces.surface IN ('webapp', 'api', 'internal')",
		"ON CONFLICT DO NOTHING",
	} {
		if !strings.Contains(seedSQL, want) {
			t.Fatalf("published runtime authorization seed SQL missing %q: %s", want, seedSQL)
		}
	}
}

func compactSQL(input string) string {
	return strings.Join(strings.Fields(input), " ")
}
