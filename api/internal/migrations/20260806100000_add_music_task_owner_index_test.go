package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestMusicTaskOwnerIndexMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*CREATE INDEX IF NOT EXISTS idx_music_tasks_owner_created.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddMusicTaskOwnerIndex(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	want := "ON public.music_generation_tasks (organization_id, workspace_id, account_id, created_at DESC, id DESC)"
	if !strings.Contains(statements, want) {
		t.Fatalf("music task owner index migration missing %q:\n%s", want, statements)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
