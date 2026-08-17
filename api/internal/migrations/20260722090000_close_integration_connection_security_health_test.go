package migrations

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCloseIntegrationConnectionSecurityHealthMigrationExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.integration_connections.*CREATE INDEX idx_integration_oauth_states_expiry").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260722090000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestConnectionSecurityHealthMigrationKeepsManagementAuthScopeAndHealthOrthogonal(t *testing.T) {
	sql := compactSQL(closeIntegrationConnectionSecurityHealthSQL)
	for _, want := range []string{
		"ADD COLUMN health_status varchar(32) NOT NULL DEFAULT 'unknown'",
		"ADD COLUMN auth_status varchar(32) NOT NULL DEFAULT 'unknown'",
		"ADD COLUMN auth_method_id varchar(128)",
		"UPDATE public.integration_connections SET auth_method_id = auth_type WHERE auth_method_id IS NULL",
		"ALTER COLUMN auth_method_id SET NOT NULL",
		"ADD COLUMN scope_status varchar(32) NOT NULL DEFAULT 'unknown'",
		"CHECK (health_status IN ('unknown', 'healthy', 'degraded', 'unhealthy'))",
		"CHECK (auth_status IN ('unknown', 'valid', 'reconnect_required', 'expired'))",
		"CHECK (scope_status IN ('unknown', 'verified', 'drifted'))",
		"ADD COLUMN health_revision integer NOT NULL DEFAULT 1",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("connection health schema missing %q: %s", want, sql)
		}
	}
}

func TestConnectionSecurityMigrationDefinesOwnershipGrantsAndAIChatPreferences(t *testing.T) {
	sql := compactSQL(closeIntegrationConnectionSecurityHealthSQL)
	for _, want := range []string{
		"CHECK (credential_source IN ('platform', 'organization', 'account'))",
		"CHECK (auth_type IN ('platform', 'api_key', 'oauth2', 'custom_credential', 'service_account'))",
		"credential_source = 'account' AND owner_account_id IS NOT NULL",
		"CREATE TABLE public.integration_connection_grants",
		"CHECK (principal_type IN ('organization', 'workspace', 'account'))",
		"CHECK (access_mode IN ('read', 'write'))",
		"allowed_action_ids jsonb NOT NULL DEFAULT '[]'::jsonb",
		"AND NOT (allowed_action_ids ? '*')",
		"resource_constraints jsonb NOT NULL DEFAULT '{}'::jsonb",
		"CREATE TABLE public.aichat_integration_preferences",
		"selected_connection_ids jsonb NOT NULL DEFAULT '[]'::jsonb",
		"preferred_connection_id uuid NOT NULL REFERENCES public.integration_connections(id) ON DELETE CASCADE",
		"CREATE UNIQUE INDEX idx_aichat_integration_preferences_identity",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("connection authorization schema missing %q: %s", want, sql)
		}
	}
}

func TestConnectionHealthMigrationPersistsSanitizedManualAndRuntimeEvents(t *testing.T) {
	sql := strings.ToLower(compactSQL(closeIntegrationConnectionSecurityHealthSQL))
	for _, want := range []string{
		"create table public.integration_connection_health_events",
		"error_fingerprint varchar(64)",
		"create unique index idx_integration_connection_health_events_execution",
		"create table public.integration_oauth_states",
		"state_digest varchar(64) not null unique",
		"encrypted_verifier text not null",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("connection health schema missing %q: %s", want, sql)
		}
	}
	for _, forbidden := range []string{
		"raw_error",
		"error_message",
		"access_token text",
		"refresh_token text",
		"code_verifier text",
		"client_secret text",
		"api_key text",
		"integration_connection_health_jobs",
		"next_health_check_at",
		"'scheduled'",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("security/health schema must not persist %q: %s", forbidden, sql)
		}
	}
}

func TestConnectionSecurityHealthRollbackGuardsIncompatibleConnectionsBeforeDestructiveDDL(t *testing.T) {
	sql := strings.ToLower(compactSQL(rollbackIntegrationConnectionSecurityHealthSQL))
	lockIndex := strings.Index(sql, "lock table public.integration_connections in access exclusive mode")
	guardIndex := strings.Index(sql, "cannot roll back integration connection security schema")
	dropIndex := strings.Index(sql, "drop table if exists public.integration_oauth_states")
	if lockIndex < 0 || guardIndex < 0 || dropIndex < 0 || lockIndex > guardIndex || guardIndex > dropIndex {
		t.Fatalf("rollback guard must hold an exclusive lock and fail before destructive DDL: %s", sql)
	}
	for _, want := range []string{
		"where not (",
		"credential_source = 'platform' and auth_type = 'platform' and encrypted_credentials is null",
		"credential_source = 'organization' and auth_type = 'api_key' and encrypted_credentials is not null and char_length(encrypted_credentials) > 3",
		"account-owned",
		"oauth2",
		"custom-credential",
		"service-account",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("rollback guard missing %q: %s", want, sql)
		}
	}
}

func TestConnectionSecurityHealthRollbackFailsClosedAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run PostgreSQL rollback guard test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	if err := RunWithDB(db); err != nil {
		t.Fatalf("prepare migrated schema: %v", err)
	}

	t.Run("incompatible data blocks rollback without dropping schema", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin fixture transaction: %v", tx.Error)
		}
		t.Cleanup(func() { _ = tx.Rollback().Error })
		organizationID, accountID, connectionID := insertRollbackGuardIdentityFixtures(t, tx)
		mustExecWithArgs(t, tx, `
			INSERT INTO public.integration_connections (
				id, organization_id, integration_id, driver_id, name,
				credential_source, auth_type, auth_method_id, encrypted_credentials,
				owner_account_id, config, granted_scopes, status
			) VALUES (?, ?, 'github', 'github-rest', 'Personal rollback guard',
				'account', 'api_key', 'pat_personal', 'encrypted-fixture',
				?, '{}'::jsonb, '[]'::jsonb, 'active')
		`, connectionID, organizationID, accountID)
		mustExec(t, tx, "SAVEPOINT before_guarded_down")
		if err := down20260722090000(mschema.New(tx).AllowDestructive()); err == nil {
			t.Fatal("rollback unexpectedly accepted account-owned connection")
		}
		mustExec(t, tx, "ROLLBACK TO SAVEPOINT before_guarded_down")

		assertMigrationColumnExists(t, tx, "integration_connections", "owner_account_id", true)
		assertMigrationTableExists(t, tx, "integration_connection_grants", true)
		var count int64
		if err := tx.Table("integration_connections").Where("id = ?", connectionID).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("guarded rollback changed fixture: count=%d err=%v", count, err)
		}
	})

	t.Run("legacy-compatible data can roll back to restrictive schema", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin fixture transaction: %v", tx.Error)
		}
		t.Cleanup(func() { _ = tx.Rollback().Error })
		organizationID, _, connectionID := insertRollbackGuardIdentityFixtures(t, tx)
		// The test DSN is expected to be disposable. Within this transaction,
		// remove incompatible rows so the success path can exercise the old
		// constraints; the outer rollback restores all data and schema.
		mustExec(t, tx, `DELETE FROM public.integration_connections WHERE NOT (
			(credential_source = 'platform' AND auth_type = 'platform' AND encrypted_credentials IS NULL)
			OR
			(credential_source = 'organization' AND auth_type = 'api_key' AND encrypted_credentials IS NOT NULL AND char_length(encrypted_credentials) > 3)
		)`)
		mustExecWithArgs(t, tx, `
			INSERT INTO public.integration_connections (
				id, organization_id, integration_id, driver_id, name,
				credential_source, auth_type, auth_method_id, encrypted_credentials,
				config, granted_scopes, status
			) VALUES (?, ?, 'github', 'github-rest', 'Organization rollback guard',
				'organization', 'api_key', 'pat_organization', 'encrypted-fixture',
				'{}'::jsonb, '[]'::jsonb, 'active')
		`, connectionID, organizationID)
		if err := down20260722090000(mschema.New(tx).AllowDestructive()); err != nil {
			t.Fatalf("rollback rejected legacy-compatible data: %v", err)
		}
		assertMigrationColumnExists(t, tx, "integration_connections", "owner_account_id", false)
		assertMigrationTableExists(t, tx, "integration_connection_grants", false)

		mustExec(t, tx, "SAVEPOINT before_invalid_legacy_insert")
		err := tx.Exec(`
			INSERT INTO public.integration_connections (
				id, organization_id, integration_id, driver_id, name,
				credential_source, auth_type, encrypted_credentials, config, granted_scopes, status
			) VALUES (?, ?, 'github', 'github-rest', 'Rejected account',
				'account', 'api_key', 'encrypted-fixture', '{}'::jsonb, '[]'::jsonb, 'active')
		`, uuid.New(), organizationID).Error
		if err == nil {
			t.Fatal("legacy credential constraint accepted account-owned connection")
		}
		mustExec(t, tx, "ROLLBACK TO SAVEPOINT before_invalid_legacy_insert")
	})
}

func insertRollbackGuardIdentityFixtures(t *testing.T, tx *gorm.DB) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	organizationID, accountID, connectionID := uuid.New(), uuid.New(), uuid.New()
	mustExecWithArgs(t, tx, `INSERT INTO public.accounts (id, name, email) VALUES (?, 'Rollback Guard', ?)`, accountID, fmt.Sprintf("rollback-%s@example.invalid", accountID))
	mustExecWithArgs(t, tx, `INSERT INTO public.organizations (id, name) VALUES (?, ?)`, organizationID, "Rollback Guard "+organizationID.String())
	return organizationID, accountID, connectionID
}

func mustExecWithArgs(t *testing.T, db *gorm.DB, statement string, args ...interface{}) {
	t.Helper()
	if err := db.Exec(statement, args...).Error; err != nil {
		t.Fatalf("exec SQL failed: %v", err)
	}
}

func assertMigrationColumnExists(t *testing.T, db *gorm.DB, table, column string, want bool) {
	t.Helper()
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`, table, column).Scan(&count).Error; err != nil {
		t.Fatalf("inspect column %s.%s: %v", table, column, err)
	}
	if (count == 1) != want {
		t.Fatalf("column %s.%s exists=%t, want %t", table, column, count == 1, want)
	}
}

func assertMigrationTableExists(t *testing.T, db *gorm.DB, table string, want bool) {
	t.Helper()
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ?`, table).Scan(&count).Error; err != nil {
		t.Fatalf("inspect table %s: %v", table, err)
	}
	if (count == 1) != want {
		t.Fatalf("table %s exists=%t, want %t", table, count == 1, want)
	}
}
