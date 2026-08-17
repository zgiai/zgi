package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestStandardizeGraphFlowLLMExtractionMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectBegin()
	mock.ExpectExec("(?s).*").WithArgs("llm", "llm").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	builder := mschema.New(db)
	if err := up202608040452036859(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{
		"ALTER COLUMN extraction_strategy SET DEFAULT 'llm'",
		"DATA FIX: standardize dataset graph extraction strategy to llm",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("LLM extraction migration missing %q:\n%s", required, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
