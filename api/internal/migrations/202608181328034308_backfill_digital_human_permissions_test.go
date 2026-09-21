package migrations

import (
	"strings"
	"testing"
)

func TestBackfillDigitalHumanPermissionsOnlyUpdatesSystemRoleSnapshots(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(backfillDigitalHumanPermissionsSQL), " "))
	for _, required := range []string{
		"template_origin = 'system_default'",
		"default_advanced",
		"default_basic",
		"default_readonly",
		"digital_human.publish",
		"digital_human.run",
		"permission_source in ('role_template', 'legacy_role')",
		"jsonb_agg(distinct expanded.permission",
	} {
		if !strings.Contains(compact, required) {
			t.Errorf("permission backfill is missing %q", required)
		}
	}
	if strings.Contains(compact, "permission_source = 'direct'") {
		t.Fatal("permission backfill must not expand direct member permissions")
	}

	down := strings.ToLower(strings.Join(strings.Fields(removeBackfilledDigitalHumanPermissionsSQL), " "))
	if !strings.Contains(down, "not like 'digital_human.%'") ||
		!strings.Contains(down, "template_origin = 'system_default'") {
		t.Fatal("permission rollback must remove only DigitalHuman permissions from system snapshots")
	}
}
