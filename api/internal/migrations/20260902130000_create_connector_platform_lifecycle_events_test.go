package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestConnectorPlatformLifecycleEventMigrationIsImmutableAndSecretFree(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.connector_platform_lifecycle_events.*CREATE TRIGGER connector_platform_lifecycle_events_immutable").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260902130000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}

	mock.ExpectExec("(?s)cannot remove Connector platform lifecycle audit while events exist.*DROP TABLE IF EXISTS public.connector_platform_lifecycle_events").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260902130000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	sql := strings.Join(strings.Fields(createConnectorPlatformLifecycleEventsSQL), " ")
	for _, required := range []string{
		"actor_kind IN ('system', 'platform_maintainer')",
		"actor_kind <> 'platform_maintainer' OR actor_id IS NOT NULL",
		"status = 'successful'",
		"UNIQUE (subject_type, subject_id, correlation_id)",
		"EXECUTE FUNCTION public.reject_connector_immutable_mutation()",
		"jsonb_typeof(details) = 'object'",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration SQL is missing %q", required)
		}
	}
	for _, forbidden := range []string{"encrypted_credentials", "provider_key", "credential_value"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("platform lifecycle table contains secret-bearing column %q", forbidden)
		}
	}
}
