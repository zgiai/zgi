package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestSerializeGraphFlowRunsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upSerializeGraphFlowRuns(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{
		"PARTITION BY dataset_id",
		"SET status = 'pending'",
		"idx_graphflow_runs_one_processing_per_dataset",
		"WHERE status = 'processing'",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("run serialization migration missing %q:\n%s", required, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
