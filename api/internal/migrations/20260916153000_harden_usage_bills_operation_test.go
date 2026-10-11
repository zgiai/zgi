package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestHardenUsageBillsOperationMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s).*ALTER COLUMN operation SET DEFAULT ''`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s).*SET operation = '' WHERE operation IS NULL`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s).*ALTER COLUMN operation SET NOT NULL`).WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upHardenUsageBillsOperation(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	if !strings.Contains(statements, "SET DEFAULT ''") {
		t.Fatalf("hardening migration missing default: \n%s", statements)
	}
	if !strings.Contains(statements, "SET NOT NULL") {
		t.Fatalf("hardening migration missing not-null: \n%s", statements)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHardenUsageBillsOperationRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s).*ALTER COLUMN operation DROP DEFAULT`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s).*ALTER COLUMN operation DROP NOT NULL`).WillReturnResult(sqlmock.NewResult(0, 0))

	if err := downHardenUsageBillsOperation(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
