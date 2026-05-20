package migrationsv2

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestM0034AddAIChatConversationMetadata(t *testing.T) {
	db, mock, cleanup := openMigrationMockDB(t)
	defer cleanup()

	mock.ExpectExec(`ALTER TABLE public\.aichat_conversations ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '\{\}'::jsonb`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0034_add_aichat_conversation_metadata().Migrate(db); err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestM0034AddAIChatConversationMetadataRollback(t *testing.T) {
	db, mock, cleanup := openMigrationMockDB(t)
	defer cleanup()

	mock.ExpectExec(`ALTER TABLE public\.aichat_conversations DROP COLUMN IF EXISTS metadata`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0034_add_aichat_conversation_metadata().Rollback(db); err != nil {
		t.Fatalf("rollback returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
