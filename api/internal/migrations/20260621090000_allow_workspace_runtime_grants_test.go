package migrations

import (
	"strings"
	"testing"
)

func TestAllowWorkspaceRuntimeGrantsMigrationExtendsSubjectCheck(t *testing.T) {
	sql := compactSQL(allowWorkspacePublishedRuntimeGrantSubjectsSQL)
	for _, want := range []string{
		"ALTER TABLE public.published_runtime_surface_grants DROP CONSTRAINT IF EXISTS published_runtime_surface_grants_subject_check",
		"ADD CONSTRAINT published_runtime_surface_grants_subject_check CHECK (subject_type IN ('public', 'organization', 'department', 'workspace', 'account', 'internal'))",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("workspace runtime grants migration SQL missing %q: %s", want, sql)
		}
	}
}
