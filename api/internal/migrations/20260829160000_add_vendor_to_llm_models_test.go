package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddVendorToLLMModelsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_models.*ADD COLUMN IF NOT EXISTS vendor varchar\\(100\\).*CREATE INDEX IF NOT EXISTS idx_llm_models_vendor.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddVendorToLLMModels(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS vendor varchar(100) NOT NULL DEFAULT ''",
		"CREATE INDEX IF NOT EXISTS idx_llm_models_vendor",
	} {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("vendor migration is incomplete; missing %q:\n%s", fragment, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
