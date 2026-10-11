package migrations

import (
	"strings"
	"testing"
)

func TestAddDefaultSubjectToChannelInstallationsMigrationGuardsScope(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(addDefaultSubjectToChannelInstallationsSQL), " "))
	required := []string{
		"add column if not exists default_subject_type varchar(32)",
		"add column if not exists default_subject_id uuid",
		"channel_installations_default_subject_check",
		"default_subject_type = 'digital_human'",
		"enforce_channel_installation_default_subject",
		"subject.organization_id = new.organization_id",
		"subject.workspace_id = new.workspace_id",
		"subject.deleted_at is null",
	}
	for _, token := range required {
		if !strings.Contains(compact, token) {
			t.Errorf("migration is missing %q", token)
		}
	}
}
