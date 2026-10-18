package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddDigitalHumanVersionReleaseMetadataMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.digital_human_versions.*ADD COLUMN IF NOT EXISTS name character varying\\(80\\).*ADD COLUMN IF NOT EXISTS description text.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddDigitalHumanVersionReleaseMetadata(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS name character varying(80) NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT ''",
	} {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("digital human version release metadata migration is incomplete; missing %q:\n%s", fragment, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
