package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddCustomModelCachePricesMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_custom_models.*cost_cache_read.*cost_cache_write.*cache_read_price_configured.*cache_write_price_configured.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddCustomModelCachePrices(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, column := range []string{
		"cost_cache_read", "cost_cache_write", "cache_read_price_configured", "cache_write_price_configured",
	} {
		if !strings.Contains(statements, column) {
			t.Fatalf("custom model cache price migration missing %q:\n%s", column, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
