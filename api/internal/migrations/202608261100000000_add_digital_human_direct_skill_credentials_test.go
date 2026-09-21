package migrations

import (
	"strings"
	"testing"
)

func TestDigitalHumanDirectSkillCredentialMigrationStoresOnlyEncryptedMaterial(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(addDigitalHumanDirectSkillCredentialsSQL), " "))
	for _, required := range []string{
		"create table if not exists public.digital_human_direct_skill_credentials",
		"encrypted_credentials text not null",
		"digital_human_id uuid not null references public.digital_humans(id) on delete cascade",
		"unique index if not exists idx_digital_human_direct_skill_identity",
		"credential_version integer not null",
		"revision bigint not null",
	} {
		if !strings.Contains(compact, required) {
			t.Errorf("migration is missing %q", required)
		}
	}
	for _, forbidden := range []string{"api_token", "api_key", "access_token", "client_secret"} {
		if strings.Contains(compact, forbidden) {
			t.Errorf("migration exposes credential field %q", forbidden)
		}
	}
}
