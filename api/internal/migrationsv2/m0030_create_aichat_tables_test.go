package migrationsv2

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestM0030CreateAIChatTablesIncludesRuntimeFields(t *testing.T) {
	db, mock, cleanup := openMigrationMockDB(t)
	defer cleanup()

	expectations := []string{
		`CREATE TABLE IF NOT EXISTS public\.aichat_conversations[\s\S]*id UUID PRIMARY KEY,[\s\S]*runtime_status VARCHAR\(32\) NOT NULL DEFAULT 'idle'[\s\S]*active_message_id UUID`,
		`CREATE TABLE IF NOT EXISTS public\.aichat_messages[\s\S]*id UUID PRIMARY KEY,`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_owner_updated`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_workspace`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_runtime_status\s+ON public\.aichat_conversations\(runtime_status\)`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_active_message\s+ON public\.aichat_conversations\(active_message_id\)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_aichat_conversations_source_conversation`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_source_web_app`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_messages_conversation_created`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_messages_parent`,
		`CREATE INDEX IF NOT EXISTS idx_aichat_messages_billing_reason_source`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_aichat_messages_source_message`,
	}
	for _, expectation := range expectations {
		mock.ExpectExec(expectation).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	if err := M0030_create_aichat_tables().Migrate(db); err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestM0030CreateAIChatTablesRollbackDropsRuntimeIndexes(t *testing.T) {
	db, mock, cleanup := openMigrationMockDB(t)
	defer cleanup()

	expectations := []string{
		`DROP INDEX IF EXISTS public\.idx_aichat_messages_source_message`,
		`DROP INDEX IF EXISTS public\.idx_aichat_messages_billing_reason_source`,
		`DROP INDEX IF EXISTS public\.idx_aichat_messages_parent`,
		`DROP INDEX IF EXISTS public\.idx_aichat_messages_conversation_created`,
		`DROP INDEX IF EXISTS public\.idx_aichat_conversations_source_web_app`,
		`DROP INDEX IF EXISTS public\.idx_aichat_conversations_source_conversation`,
		`DROP INDEX IF EXISTS public\.idx_aichat_conversations_active_message`,
		`DROP INDEX IF EXISTS public\.idx_aichat_conversations_runtime_status`,
		`DROP INDEX IF EXISTS public\.idx_aichat_conversations_workspace`,
		`DROP INDEX IF EXISTS public\.idx_aichat_conversations_owner_updated`,
		`DROP TABLE IF EXISTS public\.aichat_messages`,
		`DROP TABLE IF EXISTS public\.aichat_conversations`,
	}
	for _, expectation := range expectations {
		mock.ExpectExec(expectation).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	if err := M0030_create_aichat_tables().Rollback(db); err != nil {
		t.Fatalf("rollback returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
