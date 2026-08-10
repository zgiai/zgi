package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddLLMInvocationContentGovernanceMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*llm_content_retention_days.*ck_organizations_llm_content_retention_days.*llm_invocation_content_views.*action.*idx_llm_invocation_contents_expires.*").WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upAddLLMInvocationContentGovernance(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, expected := range []string{
		"llm_content_retention_days",
		"ck_organizations_llm_content_retention_days",
		"llm_invocation_content_views",
		"action",
		"idx_llm_invocation_contents_expires",
	} {
		if !strings.Contains(statements, expected) {
			t.Fatalf("migration statements missing %q:\n%s", expected, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
