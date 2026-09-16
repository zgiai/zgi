package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDropPriceConfiguredBooleansMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_models.*DROP COLUMN IF EXISTS input_price_configured.*ALTER TABLE public.llm_custom_models.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db).AllowDestructive()
	if err := upDropPriceConfiguredBooleans(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, column := range []string{
		"input_price_configured",
		"output_price_configured",
		"cache_read_price_configured",
		"cache_write_price_configured",
		"cache_write_5m_price_configured",
		"cache_write_1h_price_configured",
	} {
		if !strings.Contains(statements, column) {
			t.Fatalf("drop configured migration missing %q:\n%s", column, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
