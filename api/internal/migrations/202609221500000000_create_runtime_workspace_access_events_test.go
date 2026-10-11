package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateRuntimeWorkspaceAccessEventsAddsScopedAuditSink(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	for range 6 {
		mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 0))
	}

	if err := upCreateRuntimeWorkspaceAccessEvents(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRuntimeWorkspaceAccessEventsRollbackIsDestructiveOnlyWhenAllowed(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*DROP TABLE IF EXISTS.*").WillReturnResult(sqlmock.NewResult(0, 0))

	if err := downCreateRuntimeWorkspaceAccessEvents(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
