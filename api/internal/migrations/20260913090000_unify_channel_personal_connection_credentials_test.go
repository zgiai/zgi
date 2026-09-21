package migrations

import (
	"strings"
	"testing"
)

func TestChannelPersonalConnectionCredentialMigrationContract(t *testing.T) {
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS connector_id",
		"ADD COLUMN IF NOT EXISTS connection_owner_id",
		"ADD COLUMN IF NOT EXISTS credential_source",
		"ADD COLUMN IF NOT EXISTS connection_revision",
		"ADD COLUMN IF NOT EXISTS connection_credential_version",
		"channel_installations_credential_source_check",
		"channel_installations_credential_locator_check",
		"channel_installations_connection_owner_id_fkey",
		"idx_channel_installations_personal_connection",
		"connection_id = id",
		"connection_id <> id",
		"connection_owner_id = COALESCE(connection_owner_id, created_by)",
		"connector_id = COALESCE(NULLIF(connector_id, ''), lower(provider))",
	} {
		if !strings.Contains(unifyChannelPersonalConnectionCredentialsSQL, fragment) && !strings.Contains(backfillChannelPersonalConnectionCredentialSQL, fragment) {
			t.Fatalf("migration SQL missing %q", fragment)
		}
	}
	if strings.Contains(unifyChannelPersonalConnectionCredentialsSQL+backfillChannelPersonalConnectionCredentialSQL, "encrypted_credentials") {
		t.Fatal("channel installation migration must not copy credential material")
	}
}
