package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestAddWorkflowRuntimeV2Migration(t *testing.T) {
	var migrate func(*gorm.DB) error
	for _, candidate := range registeredMigrations() {
		if candidate.ID == migrationAddWorkflowRuntimeV2ID {
			migrate = candidate.Migrate
			break
		}
	}
	if migrate == nil {
		t.Fatalf("migration %s is not registered", migrationAddWorkflowRuntimeV2ID)
	}

	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.workflow_run_logs.*runtime_protocol_version.*CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_run_events_tenant_run_sequence_unique.*CREATE TABLE IF NOT EXISTS public.workflow_runtime_outbox.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
