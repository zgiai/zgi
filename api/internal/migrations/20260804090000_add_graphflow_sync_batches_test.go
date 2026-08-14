package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddGraphFlowSyncBatchesMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddGraphFlowSyncBatches(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS sync_batch_id uuid",
		"CREATE TABLE IF NOT EXISTS public.graphflow_run_items",
		"idx_graphflow_run_items_operation_document",
		"ADD COLUMN IF NOT EXISTS run_item_id uuid",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("sync batch migration missing %q:\n%s", required, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
