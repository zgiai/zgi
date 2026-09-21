package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestConnectorMigrationRolloutStateMigrationExecutesAndGuardsRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.connector_migration_inventory.*CREATE TABLE public.connector_rollout_evidence.*connector_rollout_evidence_immutable").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260902110000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}

	mock.ExpectExec("(?s)cannot remove Connector migration rollout state while evidence exists.*DROP TABLE IF EXISTS public.connector_migration_inventory").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260902110000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorMigrationRolloutSchemaPersistsFailClosedEvidence(t *testing.T) {
	sql := compactSQL(createConnectorMigrationRolloutStateSQL)
	for _, expected := range []string{
		"CREATE TABLE public.connector_migration_inventory",
		"CREATE TABLE public.connector_provider_rollouts",
		"CREATE TABLE public.connector_shadow_observations",
		"CREATE TABLE public.connector_rollout_evidence",
		"UNIQUE (connector_id, rollout_revision, organization_id, logical_operation_id)",
		"UNIQUE (connector_id, rollout_revision, evidence_type, correlation_id)",
		"eligibility IN ('eligible', 'blocked', 'excluded', 'backfilled')",
		"phase IN ('legacy', 'shadow', 'canary', 'connector', 'reversing')",
		"live_path = 'legacy'",
		"connector_shadow_observations_immutable",
		"connector_rollout_evidence_immutable",
	} {
		if !strings.Contains(sql, compactSQL(expected)) {
			t.Fatalf("migration SQL missing %q", expected)
		}
	}
}

func TestConnectorMigrationInventoryCannotPersistCredentials(t *testing.T) {
	lower := strings.ToLower(createConnectorMigrationRolloutStateSQL)
	inventoryStart := strings.Index(lower, "create table public.connector_migration_inventory")
	inventoryEnd := strings.Index(lower[inventoryStart:], "create table public.connector_provider_rollouts")
	if inventoryStart < 0 || inventoryEnd < 0 {
		t.Fatal("inventory table boundary missing")
	}
	inventory := lower[inventoryStart : inventoryStart+inventoryEnd]
	for _, forbidden := range []string{"encrypted_credentials", "credential_value", "api_key", "token_value", "secret_value"} {
		if strings.Contains(inventory, forbidden) {
			t.Fatalf("migration inventory contains secret-bearing column %q", forbidden)
		}
	}
}
