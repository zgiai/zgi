package migrations

import (
	"strings"
	"testing"
)

func TestDigitalHumanRolesAndBindingsMigrationPinsImmutableVersions(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(createDigitalHumanRolesAndBindingsSQL), " "))
	required := []string{
		"create table if not exists public.digital_human_role_templates",
		"create table if not exists public.digital_human_role_template_versions",
		"create table if not exists public.digital_human_role_assignments",
		"create table if not exists public.digital_human_resource_bindings",
		"digital_human_version_id uuid not null",
		"role_template_version_id uuid not null",
		"published digital human role assignments are immutable",
		"published digital human resource bindings are immutable",
		"credential_ref varchar(512) not null default ''",
		"resource_revision varchar(255) not null",
		"binding_revision varchar(71) not null",
	}
	for _, token := range required {
		if !strings.Contains(compact, token) {
			t.Errorf("migration is missing %q", token)
		}
	}
	for _, forbidden := range []string{"agent_id", "references public.agents", "access_token", "api_key", "password"} {
		if strings.Contains(compact, forbidden) {
			t.Errorf("migration contains forbidden token %q", forbidden)
		}
	}
}
