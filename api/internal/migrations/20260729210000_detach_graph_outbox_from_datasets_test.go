package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDetachGraphOutboxFromDatasetsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upDetachGraphOutboxFromDatasets(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	if !strings.Contains(statements, "DROP CONSTRAINT IF EXISTS graph_outbox_events_dataset_id_fkey") {
		t.Fatalf("dataset purge events must survive dataset deletion:\n%s", statements)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
