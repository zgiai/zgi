package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddRuntimeRequestLeasesIsIdempotent(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*ADD COLUMN IF NOT EXISTS in_flight_requests bigint NOT NULL DEFAULT 0.*DO \$\$.*ck_runtime_workspace_in_flight_requests`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upAddRuntimeRequestLeases(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddRuntimeRequestLeasesRollbackDropsColumnAndConstraint(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.runtime_workspaces.*DROP CONSTRAINT IF EXISTS ck_runtime_workspace_in_flight_requests.*DROP COLUMN IF EXISTS in_flight_requests`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := downAddRuntimeRequestLeases(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
