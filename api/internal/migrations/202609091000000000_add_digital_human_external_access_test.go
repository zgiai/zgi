package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddDigitalHumanExternalAccessSeparatesSchemaAndDataFixes(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.digital_humans.*CREATE INDEX IF NOT EXISTS idx_digital_human_api_keys_owner`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)UPDATE public\.digital_humans.*web_app_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)UPDATE public\.digital_human_runs.*principal_type = 'account'`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)UPDATE public\.digital_human_runs.*COALESCE\(NULLIF`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)ALTER TABLE public\.digital_humans.*SET NOT NULL.*CREATE UNIQUE INDEX`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddDigitalHumanExternalAccess(builder); err != nil {
		t.Fatal(err)
	}

	statements := builder.Statements()
	if len(statements) != 3 {
		t.Fatalf("unexpected migration statement count: got %d, want 3\n%s", len(statements), strings.Join(statements, "\n"))
	}
	if strings.Contains(strings.ToUpper(statements[0]), "UPDATE ") {
		t.Fatalf("schema DDL must not contain data updates:\n%s", statements[0])
	}
	if statements[1] != "DATA FIX: backfill DigitalHuman external access identity" {
		t.Fatalf("migration must record the external access data fix, got %q", statements[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
