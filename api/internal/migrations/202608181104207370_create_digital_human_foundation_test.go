package migrations

import (
	"strings"
	"testing"
)

func TestDigitalHumanFoundationMigrationDefinesIndependentVersionedDomain(t *testing.T) {
	t.Parallel()

	sql := compactSQL(createDigitalHumanFoundationSQL)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS public.digital_humans",
		"organization_id uuid NOT NULL REFERENCES public.organizations(id)",
		"workspace_id uuid NOT NULL REFERENCES public.workspaces(id)",
		"status IN ('draft', 'active', 'disabled')",
		"autonomy_level IN ('assistive', 'supervised', 'autonomous')",
		"visibility IN ('private', 'workspace')",
		"CREATE TABLE IF NOT EXISTS public.digital_human_versions",
		"digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT",
		"state IN ('draft', 'published')",
		"persona text NOT NULL",
		"rules jsonb NOT NULL",
		"default_model varchar(255) NOT NULL",
		"memory_policy jsonb NOT NULL",
		"trigger_policy jsonb NOT NULL",
		"identity_snapshot jsonb NOT NULL",
		"revision varchar(71) NOT NULL",
		"lock_version bigint NOT NULL DEFAULT 1",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_human_versions_one_draft",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_human_versions_number",
		"FOREIGN KEY (draft_version_id) REFERENCES public.digital_human_versions(id)",
		"FOREIGN KEY (published_version_id) REFERENCES public.digital_human_versions(id)",
		"CREATE TRIGGER trg_digital_human_version_scope",
		"CREATE TRIGGER trg_published_digital_human_version_immutable",
		"IF OLD.state = 'published'",
		"CREATE TRIGGER trg_digital_human_version_pointers",
		"version.digital_human_id = NEW.id",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("digital human migration missing %q: %s", want, sql)
		}
	}
	if strings.Contains(sql, "agent_id") || strings.Contains(sql, "REFERENCES public.agents") {
		t.Fatalf("digital human migration depends on Agent storage: %s", sql)
	}
}
