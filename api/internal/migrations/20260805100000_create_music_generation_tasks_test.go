package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateMusicGenerationTasksMigrationUsesApplicationUUIDsAndRequestIdempotency(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*CREATE TABLE IF NOT EXISTS public.music_generation_tasks.*id uuid PRIMARY KEY.*request_id uuid NOT NULL.*CREATE UNIQUE INDEX IF NOT EXISTS idx_music_tasks_request.*idx_music_tasks_status_updated.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upCreateMusicGenerationTasks(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	if strings.Contains(statements, "gen_random_uuid") || strings.Contains(statements, "uuid_generate") {
		t.Fatalf("migration depends on database UUID generation:\n%s", statements)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
