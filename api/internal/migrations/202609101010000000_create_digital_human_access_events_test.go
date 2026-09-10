package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateDigitalHumanAccessEventsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*CREATE TABLE IF NOT EXISTS public.digital_human_access_events.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upCreateDigitalHumanAccessEvents(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, fragment := range []string{
		"actor_id uuid REFERENCES public.accounts(id) ON DELETE SET NULL",
		"details jsonb NOT NULL DEFAULT '{}'::jsonb",
		"CHECK (result IN ('success', 'failure'))",
		"idx_digital_human_access_events_timeline",
	} {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("access audit migration is incomplete; missing %q:\n%s", fragment, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
