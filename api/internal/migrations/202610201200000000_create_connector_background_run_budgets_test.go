package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateConnectorBackgroundRunBudgetsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.connector_background_run_budgets.*CREATE INDEX idx_background_run_budgets_scope").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up202610201200000000(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DROP TABLE IF EXISTS public.connector_background_run_budgets").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down202610201200000000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
