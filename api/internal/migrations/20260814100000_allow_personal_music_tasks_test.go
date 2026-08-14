package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAllowPersonalMusicTasksMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.music_generation_tasks.*workspace_id DROP NOT NULL.*idx_music_tasks_personal_request.*WHERE workspace_id IS NULL.*idx_music_tasks_account_created.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAllowPersonalMusicTasks(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, want := range []string{
		"ALTER COLUMN workspace_id DROP NOT NULL",
		"ON public.music_generation_tasks (organization_id, account_id, request_id)",
		"WHERE workspace_id IS NULL",
		"ON public.music_generation_tasks (organization_id, account_id, created_at DESC, id DESC)",
	} {
		if !strings.Contains(statements, want) {
			t.Fatalf("personal music task migration missing %q:\n%s", want, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
