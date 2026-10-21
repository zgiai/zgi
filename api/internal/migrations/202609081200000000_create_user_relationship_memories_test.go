package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateUserRelationshipMemoriesMigrationContainsPrivacyAndCutoverFences(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*CREATE TABLE IF NOT EXISTS public.user_memory_subjects.*CREATE TABLE IF NOT EXISTS public.user_memory_extraction_jobs.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s).*archive_user_memory_consumer_relationships.*config.config_snapshot->>'agent_memory_auto_extraction_enabled'.*ON CONFLICT DO NOTHING.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upCreateUserRelationshipMemories(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{
		"UNIQUE (workspace_id, subject_id, consumer_type, consumer_id)",
		"content_encoding varchar(24)",
		"memory_epoch bigint",
		"definition_snapshot jsonb",
		"previous_content_payload text",
		"DATA FIX: install consumer lifecycle fences and migrate legacy Agent user memory",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}
	for _, forbidden := range []string{"external_user varchar", "raw_user", "email varchar", "phone varchar"} {
		if strings.Contains(statements, forbidden) {
			t.Fatalf("migration persists forbidden raw identity field %q", forbidden)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
