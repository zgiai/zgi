package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestFileExtractionCachesMigrationUsesUploadFileUUID(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	for range 6 {
		mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 0))
	}

	builder := mschema.New(db)
	if err := upCreateFileExtractionCaches(builder); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(builder.Statements(), "\n")
	for _, want := range []string{
		"id uuid NOT NULL PRIMARY KEY",
		"ALTER COLUMN id DROP DEFAULT",
		"file_id uuid NOT NULL",
		"ALTER COLUMN file_id TYPE uuid USING file_id::uuid",
		"REFERENCES public.upload_files (id)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("file extraction cache migration missing %q:\n%s", want, got)
		}
	}
	for _, wrong := range []string{
		"file_id character varying",
		"file_id varchar",
		strings.Join([]string{"uuid", "generate", "v4"}, "_"),
	} {
		if strings.Contains(got, wrong) {
			t.Fatalf("file extraction cache migration must not create file_id as text type %q:\n%s", wrong, got)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func openMigrationMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}
