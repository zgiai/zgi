package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateFileObjectDeletionsExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE IF NOT EXISTS public.file_object_deletions.*idx_file_object_deletions_ready").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upCreateFileObjectDeletions(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	mock.ExpectExec("(?s)DO \\$\\$.*cannot remove pending file object deletions while records exist.*DROP TABLE public.file_object_deletions").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := downCreateFileObjectDeletions(mschema.New(db)); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestCreateFileObjectDeletionsMigrationContract(t *testing.T) {
	sql := strings.ToLower(createFileObjectDeletionsSQL)
	for _, fragment := range []string{
		"create table if not exists public.file_object_deletions",
		"object_key varchar(1024) not null",
		"attempt_count integer not null default 0",
		"next_attempt_at timestamptz not null",
		"last_error text not null default ''",
		"index if not exists idx_file_object_deletions_ready",
		"on public.file_object_deletions (next_attempt_at, created_at)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}
	// The handoff only needs the object location: snapshotting mutable file metadata here
	// would create a second, stale source of truth.
	for _, forbidden := range []string{"file_name", "file_size", "organization_id", "created_by"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("outbox migration must not persist mutable file metadata %q", forbidden)
		}
	}
	rollback := strings.ToLower(rollbackFileObjectDeletionsSQL)
	for _, fragment := range []string{
		"if exists (",
		"from public.file_object_deletions",
		"raise exception 'cannot remove pending file object deletions while records exist'",
		"drop table public.file_object_deletions",
	} {
		if !strings.Contains(rollback, fragment) {
			t.Fatalf("fail-closed rollback is missing %q", fragment)
		}
	}

	found := false
	for _, migration := range registeredMigrations() {
		if migration.ID == migrationCreateFileObjectDeletionsID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("migration %s is not registered", migrationCreateFileObjectDeletionsID)
	}
}
