package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestIncreaseLLMModelPricePrecisionMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_models.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upIncreaseLLMModelPricePrecision(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, column := range []string{"input_price", "output_price", "cached_input_price"} {
		want := "ALTER COLUMN " + column + " TYPE numeric(10,6)"
		if !strings.Contains(statements, want) {
			t.Fatalf("price precision migration missing %q:\n%s", want, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
