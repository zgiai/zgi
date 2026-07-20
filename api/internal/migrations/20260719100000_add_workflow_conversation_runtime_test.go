package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestAddWorkflowConversationRuntimeMigration(t *testing.T) {
	var migrate func(*gorm.DB) error
	for _, candidate := range registeredMigrations() {
		if candidate.ID == migrationAddWorkflowConversationRuntimeID {
			migrate = candidate.Migrate
			break
		}
	}
	if migrate == nil {
		t.Fatalf("migration %s is not registered", migrationAddWorkflowConversationRuntimeID)
	}

	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.agents_conversations.*runtime_status.*ALTER TABLE public.workflow_run_logs.*conversation_id.*CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_run_logs_active_conversation_unique.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
