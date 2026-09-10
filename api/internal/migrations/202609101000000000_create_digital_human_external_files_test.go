package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateDigitalHumanExternalFilesMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*CREATE TABLE IF NOT EXISTS public.digital_human_external_files.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upCreateDigitalHumanExternalFiles(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, fragment := range []string{
		"file_id uuid PRIMARY KEY REFERENCES public.upload_files(id) ON DELETE CASCADE",
		"digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE CASCADE",
		"user_subject_id uuid NOT NULL",
		"CHECK (surface IN ('digital_human_api', 'digital_human_webapp'))",
		"CHECK (user_subject_kind IN ('account', 'api_end_user'))",
	} {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("external file migration is incomplete; missing %q:\n%s", fragment, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
