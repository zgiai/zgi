package migrations

import (
	"strings"
	"testing"
)

func TestAllowDigitalHumanRuntimeSurfacesMigrationExtendsExistingConstraint(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(allowDigitalHumanRuntimeSurfacesSQL), " "))
	for _, expected := range []string{
		"drop constraint if exists published_runtime_surfaces_resource_type_check",
		"add constraint published_runtime_surfaces_resource_type_check check",
		"resource_type in ('agent', 'builtin_workflow', 'digital_human')",
	} {
		if !strings.Contains(compact, expected) {
			t.Errorf("migration is missing %q", expected)
		}
	}
}
