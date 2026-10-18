package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestPromoteUserSubjectsRenamesRegistryAndIndexes(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.user_memory_subjects RENAME TO user_subjects.*").WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upPromoteUserSubjects(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{"user_subjects", "idx_user_subject_account", "idx_user_subject_issuer"} {
		if !strings.Contains(statements, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
