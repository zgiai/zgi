package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddDigitalHumanTaskOwnershipDefinesExclusiveOwners(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ADD COLUMN IF NOT EXISTS owner_kind.*").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s).*UPDATE .*automation_tasks.*owner_kind.*").
		WithArgs("digital_human", "digital_human_console", "digital_human").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("(?s).*ADD CONSTRAINT automation_tasks_owner_check.*").WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upAddDigitalHumanTaskOwnership(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{
		"owner_subject_id", "owner_conversation_id", "origin_surface",
		"owner_kind = 'digital_human'", "owner_kind = 'user_subject'", "owner_kind = 'conversation'",
		"REFERENCES public.user_subjects(id)", "idx_automation_tasks_user_operation", "idx_automation_tasks_conversation_operation",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
