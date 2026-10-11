package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddModelConfigPricingOverrideMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_model_configs.*ADD COLUMN IF NOT EXISTS pricing_override jsonb.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddModelConfigPricingOverride(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	if !strings.Contains(statements, "pricing_override jsonb") {
		t.Fatalf("pricing override migration missing pricing_override column:\n%s", statements)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDropModelConfigPricingOverrideMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_model_configs.*DROP COLUMN IF EXISTS pricing_override.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db).AllowDestructive()
	if err := downAddModelConfigPricingOverride(builder); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
