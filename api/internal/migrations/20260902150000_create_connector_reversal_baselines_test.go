package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestConnectorReversalBaselineMigrationExecutesAndGuardsRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.connector_reversal_baselines.*connector_reversal_baselines_immutable").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260902150000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}

	mock.ExpectExec("(?s)cannot remove Connector reversal baselines while snapshots exist.*DROP TABLE IF EXISTS public.connector_reversal_baselines").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260902150000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorReversalBaselineMigrationIsRegistered(t *testing.T) {
	for _, migration := range registeredMigrations() {
		if migration.ID == migration20260902150000ID {
			if migration.Migrate == nil || migration.Rollback == nil {
				t.Fatal("reversal baseline migration must register both directions")
			}
			return
		}
	}
	t.Fatalf("migration %q was not registered", migration20260902150000ID)
}

func TestConnectorReversalBaselineSchemaIsProtectedImmutableAndIdentityBound(t *testing.T) {
	sql := compactSQL(createConnectorReversalBaselinesSQL)
	for _, expected := range []string{
		"CREATE TABLE public.connector_reversal_baselines",
		"snapshot_ref varchar(255) PRIMARY KEY",
		"connection_id uuid NOT NULL UNIQUE",
		"encrypted_credentials text NOT NULL",
		"baseline_bindings jsonb NOT NULL",
		"FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id)",
		"REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id) ON DELETE RESTRICT",
		"connector_reversal_baselines_legacy_fk",
		"credential_source = 'account'",
		"jsonb_typeof(baseline_bindings) = 'array'",
		"connector_reversal_baselines_immutable",
	} {
		if !strings.Contains(sql, compactSQL(expected)) {
			t.Fatalf("migration SQL missing %q", expected)
		}
	}

	lower := strings.ToLower(createConnectorReversalBaselinesSQL)
	for _, forbidden := range []string{"plaintext", "credential_value", "api_key", "token_value", "secret_value"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("reversal baseline schema contains forbidden material %q", forbidden)
		}
	}
}
