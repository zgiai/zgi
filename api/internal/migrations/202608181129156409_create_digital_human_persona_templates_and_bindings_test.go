package migrations

import (
	"strings"
	"testing"
)

func TestDigitalHumanPersonaTemplatesAndBindingsMigration(t *testing.T) {
	normalized := strings.ToLower(createDigitalHumanPersonaTemplatesAndBindingsSQL)
	for _, fragment := range []string{
		"create table if not exists public.digital_human_persona_templates",
		"persona text not null",
		"goals jsonb not null",
		"rules jsonb not null",
		"create table if not exists public.digital_human_resource_bindings",
		"enforce_dh_binding_scope_and_immutability",
	} {
		if !strings.Contains(normalized, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	for _, legacy := range []string{"digital_human_role_templates", "digital_human_role_template_versions", "digital_human_role_assignments"} {
		if strings.Contains(normalized, legacy) {
			t.Fatalf("migration still contains legacy object %q", legacy)
		}
	}
}
