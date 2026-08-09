package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddLLMInvocationSourceMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("(?s).*ALTER TABLE.*llm_usage_bills.*invocation_source.*").WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddLLMInvocationSource(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, expected := range []string{"invocation_source", "DEFAULT 'unknown'", "NOT NULL"} {
		if !strings.Contains(statements, expected) {
			t.Fatalf("migration statements missing %q:\n%s", expected, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
