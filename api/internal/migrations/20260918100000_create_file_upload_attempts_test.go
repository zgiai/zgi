package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateFileUploadAttemptsExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE IF NOT EXISTS public.file_upload_attempts.*idx_file_upload_attempts_file").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upCreateFileUploadAttempts(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	mock.ExpectExec("(?s)DO \\$\\$.*cannot remove active file upload attempts while records exist.*DROP TABLE public.file_upload_attempts").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := downCreateFileUploadAttempts(mschema.New(db)); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestCreateFileUploadAttemptsMigrationContract(t *testing.T) {
	sql := strings.ToLower(createFileUploadAttemptsSQL)
	for _, fragment := range []string{
		"create table if not exists public.file_upload_attempts",
		"id uuid primary key",
		"owner_id varchar(255) not null",
		"file_id varchar(255) not null",
		"object_key varchar(1024) not null",
		"check (status in ('pending', 'completed', 'claimed', 'cancelled'))",
		"on public.file_upload_attempts (status, expires_at)",
		"on public.file_upload_attempts (file_id)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}

	rollback := strings.ToLower(rollbackFileUploadAttemptsSQL)
	for _, fragment := range []string{
		"from public.file_upload_attempts",
		"where status in ('pending', 'completed', 'cancelled')",
		"raise exception 'cannot remove active file upload attempts while records exist'",
		"drop table public.file_upload_attempts",
	} {
		if !strings.Contains(rollback, fragment) {
			t.Fatalf("fail-closed rollback is missing %q", fragment)
		}
	}

	found := false
	for _, migration := range registeredMigrations() {
		if migration.ID == migrationCreateFileUploadAttemptsID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("migration %s is not registered", migrationCreateFileUploadAttemptsID)
	}
}
