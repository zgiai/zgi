package migrations

import (
	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"testing"
)

func TestConnectorBackgroundGrantsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.connector_background_grants.*CREATE TABLE public.connector_background_consumptions.*").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up202610201000000000(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("(?s)DROP TABLE IF EXISTS public.connector_background_consumptions.*DROP TABLE IF EXISTS public.connector_background_grants").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down202610201000000000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
