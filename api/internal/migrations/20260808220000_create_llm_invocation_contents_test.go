package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateLLMInvocationContentsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.organizations.*llm_invocation_contents.*llm_invocation_content_views.*").WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upCreateLLMInvocationContents(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, expected := range []string{
		"llm_content_capture_enabled",
		"llm_invocation_contents",
		"llm_invocation_content_views",
		"expires_at",
		"redaction_version",
	} {
		if !strings.Contains(statements, expected) {
			t.Fatalf("migration statements missing %q:\n%s", expected, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
