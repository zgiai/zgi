package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddAgentMemorySlotNameMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.agent_memory_slots.*ADD COLUMN IF NOT EXISTS name character varying\\(80\\).*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddAgentMemorySlotName(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	if !strings.Contains(statements, "ADD COLUMN IF NOT EXISTS name character varying(80) NOT NULL DEFAULT ''") {
		t.Fatalf("agent memory slot name migration is incomplete:\n%s", statements)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
