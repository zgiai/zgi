package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDatasetListScopeIndexMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddDatasetListScopeIndex(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, want := range []string{
		"CREATE INDEX IF NOT EXISTS idx_datasets_organization_workspace_created",
		"ON public.datasets (organization_id, workspace_id, created_at DESC)",
	} {
		if !strings.Contains(statements, want) {
			t.Fatalf("dataset list scope index migration missing %q:\n%s", want, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
