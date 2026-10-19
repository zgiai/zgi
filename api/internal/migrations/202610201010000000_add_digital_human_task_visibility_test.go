package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddDigitalHumanTaskVisibilityKeepsOwnerColumnsAndBackfills(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s).*ADD COLUMN IF NOT EXISTS visibility.*confirmed_by_type.*confirmed_by_ref.*confirmed_at.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)UPDATE public\.automation_tasks SET visibility = CASE owner_kind.*'user_subject' THEN 'requester'.*'conversation' THEN 'conversation'.*ELSE 'workspace'.*task_kind = 'digital_human' AND visibility IS NULL`).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`(?s).*ADD CONSTRAINT automation_tasks_visibility_check.*idx_automation_tasks_digital_human_visibility.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upAddDigitalHumanTaskVisibility(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{"visibility IS NULL OR visibility IN ('requester', 'conversation', 'workspace', 'restricted')", "task_kind = 'automation' AND visibility IS NULL"} {
		if !strings.Contains(statements, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}
	if strings.Contains(statements, "owner_kind DROP") || strings.Contains(statements, "DROP COLUMN IF EXISTS owner") {
		t.Fatal("migration must keep the owner columns readable for rollback")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
