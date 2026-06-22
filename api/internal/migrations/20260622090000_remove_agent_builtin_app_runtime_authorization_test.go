package migrations

import (
	"strings"
	"testing"
)

func TestRemoveAgentBuiltinAppRuntimeAuthorizationSoftDeletesOnlyAgentSurfaces(t *testing.T) {
	sql := compactSQL(removeAgentBuiltinAppRuntimeAuthorizationSQL)
	for _, want := range []string{
		"FROM public.published_runtime_surfaces",
		"WHERE resource_type = 'agent' AND surface = 'builtin_app' AND deleted_at IS NULL",
		"update public.published_runtime_surface_grants grants set deleted_at = CURRENT_TIMESTAMP",
		"update public.published_runtime_surfaces set deleted_at = CURRENT_TIMESTAMP",
		"where resource_type = 'agent' and surface = 'builtin_app' and deleted_at IS NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("remove agent builtin app migration SQL missing %q: %s", want, sql)
		}
	}
	if strings.Contains(sql, "resource_type = 'builtin_workflow'") {
		t.Fatalf("remove agent builtin app migration must not target builtin workflow rows: %s", sql)
	}
}
