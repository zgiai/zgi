package migrations

import (
	"strings"
	"testing"
)

func TestChannelInstallationCredentialMigrationSeparatesDirectCredentialsFromConnectionCenter(t *testing.T) {
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS public.channel_installation_credentials",
		"encrypted_credentials text NOT NULL",
		"installation.connection_id = NEW.installation_id",
		"DROP CONSTRAINT IF EXISTS channel_installations_connection_id_fkey",
		"DROP CONSTRAINT IF EXISTS channel_delivery_outbox_connection_id_fkey",
	} {
		if !strings.Contains(addChannelInstallationCredentialsSQL, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}
	if strings.Contains(addChannelInstallationCredentialsSQL, "credentials json") {
		t.Fatal("migration must not persist plaintext credential JSON")
	}
}
