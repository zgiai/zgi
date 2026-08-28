package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddIntegrationOAuthBrowserBindingExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.integration_oauth_flows.*ADD COLUMN browser_binding_digest.*ALTER TABLE public.integration_oauth_states.*ADD COLUMN browser_binding_digest").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260723130000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestAddIntegrationOAuthBrowserBindingSecurityContract(t *testing.T) {
	sql := strings.ToLower(compactSQL(addIntegrationOAuthBrowserBindingSQL))
	for _, expected := range []string{
		"add column browser_binding_digest varchar(64) not null",
		"update public.integration_oauth_states set status = 'consumed'",
		"update public.integration_oauth_flows set status = 'failed'",
		"encrypted_verifier = ''",
		"encrypted_flow_token = ''",
		"check (browser_binding_digest ~ '^[0-9a-f]{64}$')",
		"alter column browser_binding_digest drop default",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("OAuth browser binding migration missing %q: %s", expected, sql)
		}
	}
	if strings.Index(sql, "update public.integration_oauth_states") > strings.Index(sql, "check (browser_binding_digest") ||
		strings.Index(sql, "update public.integration_oauth_flows") > strings.Index(sql, "check (browser_binding_digest") {
		t.Fatalf("OAuth browser binding migration constrains rows before invalidating legacy flows: %s", sql)
	}

	rollback := strings.ToLower(compactSQL(rollbackIntegrationOAuthBrowserBindingSQL))
	if !strings.Contains(rollback, "cannot remove oauth browser binding while authorization flows are pending") {
		t.Fatalf("OAuth browser binding rollback is not fail-closed: %s", rollback)
	}
	if strings.Index(rollback, "raise exception") > strings.Index(rollback, "drop column") {
		t.Fatalf("OAuth browser binding rollback checks pending flows after dropping columns: %s", rollback)
	}
}
