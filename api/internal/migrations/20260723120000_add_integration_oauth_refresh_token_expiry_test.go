package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddIntegrationOAuthRefreshTokenExpiryExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.integration_connections.*CREATE INDEX idx_integration_connections_oauth_refresh_token_expiry").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260723120000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestAddIntegrationOAuthRefreshTokenExpirySecurityContract(t *testing.T) {
	sql := strings.ToLower(compactSQL(addIntegrationOAuthRefreshTokenExpirySQL))
	for _, expected := range []string{
		"add column refresh_token_expires_at timestamptz",
		"check (refresh_token_expires_at is null or auth_type = 'oauth2')",
		"idx_integration_connections_oauth_refresh_token_expiry",
		"where auth_type = 'oauth2' and deleted_at is null",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("OAuth refresh token expiry migration missing %q: %s", expected, sql)
		}
	}
	rollback := strings.ToLower(compactSQL(rollbackIntegrationOAuthRefreshTokenExpirySQL))
	if !strings.Contains(rollback, "cannot remove oauth refresh token expiry while tracked expiry metadata exists") {
		t.Fatalf("OAuth refresh token expiry rollback is not fail-closed: %s", rollback)
	}
	if strings.Index(rollback, "raise exception") > strings.Index(rollback, "drop column") {
		t.Fatalf("OAuth refresh token expiry rollback checks data after dropping the column: %s", rollback)
	}
}
