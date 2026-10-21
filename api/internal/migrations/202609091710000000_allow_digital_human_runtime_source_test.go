package migrations

import (
	"strings"
	"testing"
)

func TestAllowDigitalHumanRuntimeSourceMigrationExtendsExistingConstraint(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(allowDigitalHumanRuntimeSourceSQL), " "))
	for _, expected := range []string{
		"drop constraint if exists published_runtime_surfaces_source_check",
		"add constraint published_runtime_surfaces_source_check check",
		"'legacy_agent_fields'",
		"'grant'",
		"'system_default'",
		"'digital_human_access'",
	} {
		if !strings.Contains(compact, expected) {
			t.Errorf("migration is missing %q", expected)
		}
	}
}
