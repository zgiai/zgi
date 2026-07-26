package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestAddWorkflowInvocationContextMigration(t *testing.T) {
	var migrate func(*gorm.DB) error
	for _, candidate := range registeredMigrations() {
		if candidate.ID == migrationAddWorkflowInvocationContextID {
			migrate = candidate.Migrate
			break
		}
	}
	if migrate == nil {
		t.Fatalf("migration %s is not registered", migrationAddWorkflowInvocationContextID)
	}

	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.workflow_run_logs.*invocation_protocol_version.*parent_invocation_id.*CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_run_logs_parent_invocation_unique.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
