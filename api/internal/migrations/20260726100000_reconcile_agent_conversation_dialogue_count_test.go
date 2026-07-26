package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestReconcileAgentConversationDialogueCountMigration(t *testing.T) {
	if migrationReconcileAgentConversationDialogueCountID <= migrationAddWorkflowRuntimeV2ID {
		t.Fatalf(
			"reconciliation migration %s must run after workflow runtime migration %s",
			migrationReconcileAgentConversationDialogueCountID,
			migrationAddWorkflowRuntimeV2ID,
		)
	}

	var migrate func(*gorm.DB) error
	for _, candidate := range registeredMigrations() {
		if candidate.ID == migrationReconcileAgentConversationDialogueCountID {
			migrate = candidate.Migrate
			break
		}
	}
	if migrate == nil {
		t.Fatalf("migration %s is not registered", migrationReconcileAgentConversationDialogueCountID)
	}

	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*WITH workflow_conversations AS.*workflow_run_id IS NOT NULL.*deleted_at IS NULL.*COUNT\\(\\*\\)::integer AS dialogue_count.*UPDATE public.agents_conversations AS conversation.*SET dialogue_count = canonical_counts.dialogue_count.*dialogue_count IS DISTINCT FROM canonical_counts.dialogue_count.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
