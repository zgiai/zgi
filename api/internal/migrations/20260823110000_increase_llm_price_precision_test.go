package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestIncreaseLLMPricePrecisionMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_models.*ALTER TABLE public.llm_model_configs.*ALTER TABLE public.llm_custom_models.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upIncreaseLLMPricePrecision(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, column := range []string{
		"input_price", "output_price", "cached_input_price", "cost_cache_read", "cost_cache_write",
		"input_price_override", "output_price_override", "cache_read_price_override", "cache_write_price_override",
	} {
		want := "ALTER COLUMN " + column + " TYPE numeric(24,12)"
		if !strings.Contains(statements, want) {
			t.Fatalf("price precision migration missing %q:\n%s", want, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
