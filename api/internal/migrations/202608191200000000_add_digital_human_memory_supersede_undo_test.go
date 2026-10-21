package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDigitalHumanMemorySupersedeUndoMigrationDefinesSnapshotAndScopeGuard(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(addDigitalHumanMemorySupersedeUndoSQL), " "))
	for _, token := range []string{
		"add column if not exists superseded_snapshot jsonb",
		"enforce_digital_human_memory_supersedes_scope",
		"prior.organization_id = new.organization_id",
		"prior.workspace_id = new.workspace_id",
		"prior.digital_human_id = new.digital_human_id",
		"prior.memory_class = new.memory_class",
		"prior.scope_type = new.scope_type",
		"prior.scope_id = new.scope_id",
	} {
		if !strings.Contains(compact, token) {
			t.Errorf("migration is missing %q", token)
		}
	}
	if strings.Contains(strings.ToUpper(compact), "DROP COLUMN") {
		t.Fatal("supersede undo migration must preserve recovery snapshots during rollback")
	}
}

func TestDigitalHumanMemorySupersedeUndoMigrationExecutesUpAndDown(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ADD COLUMN IF NOT EXISTS superseded_snapshot.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up202608191200000000(mschema.New(db)); err != nil {
		t.Fatalf("up migration: %v", err)
	}

	mock.ExpectExec("(?s).*DROP TRIGGER IF EXISTS trg_digital_human_memories_supersedes_scope.*DROP FUNCTION IF EXISTS public.enforce_digital_human_memory_supersedes_scope.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down202608191200000000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
