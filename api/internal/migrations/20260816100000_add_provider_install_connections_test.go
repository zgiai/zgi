package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestProviderInstallConnectionsMigrationExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*integration_provider_install_flows.*integration_provider_authorizations.*integration_provider_suite_states.*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := up20260816100000(mschema.New(db)); err != nil {
		t.Fatalf("up20260816100000() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderInstallConnectionsMigrationScrubsLegacyWeComSecrets(t *testing.T) {
	for _, required := range []string{
		"provider_install",
		"integration_provider_install_flows",
		"integration_provider_authorizations",
		"integration_provider_suite_states",
		"driver_id = 'wecom-open-api'",
		"auth_method_id = 'organization_wecom_custom_app'",
		"encrypted_credentials = NULL",
		"auth_status = 'reconnect_required'",
	} {
		if !strings.Contains(addProviderInstallConnectionsSQL, required) {
			t.Fatalf("provider-install migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToLower(addProviderInstallConnectionsSQL), "permanent_code") ||
		strings.Contains(strings.ToLower(addProviderInstallConnectionsSQL), "suite_secret") {
		t.Fatal("provider-install migration must not contain provider secret material")
	}
}
