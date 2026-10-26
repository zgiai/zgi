package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateWorkJournalMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.employee_work_records.*CREATE TABLE public.employee_work_record_versions.*CREATE TABLE public.employee_work_operations.*CREATE INDEX idx_work_operations_scope").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up202610201300000000(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("(?s)DROP TABLE IF EXISTS public.employee_work_operations.*DROP TABLE IF EXISTS public.employee_work_record_versions.*DROP TABLE IF EXISTS public.employee_work_records").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down202610201300000000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	for _, constraint := range []string{"UNIQUE (scope_key, kind, key)", "UNIQUE (record_id, revision)", "UNIQUE (scope_key, work_key)", "UNIQUE (unresolved_fingerprint)", "attempts BETWEEN 0 AND 1", "'pending', 'executing', 'succeeded', 'failed', 'unknown'", "input text NOT NULL", "jsonb_typeof(input::jsonb) = 'object'", "octet_length(input) <= 49152", "unresolved_fingerprint IS NOT NULL", "jsonb_typeof(sources) = 'array'"} {
		if !strings.Contains(createWorkJournalSQL, constraint) {
			t.Fatalf("missing constraint %q", constraint)
		}
	}
}
