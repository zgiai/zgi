package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateLLMBillingSettingsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE \"public\"\\.\"llm_billing_settings\"").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)INSERT INTO public\\.llm_billing_settings").
		WillReturnResult(sqlmock.NewResult(0, 1))

	builder := mschema.New(db)
	if err := up202609171626362410(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, expected := range []string{
		"\"key\" varchar(64) PRIMARY KEY",
		"\"value\" text NOT NULL",
		"\"updated_at\" timestamptz DEFAULT now() NOT NULL",
		"ON CONFLICT (\"key\") DO NOTHING",
	} {
		if !strings.Contains(statements, expected) {
			t.Fatalf("llm_billing_settings migration missing %q:\n%s", expected, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDropLLMBillingSettingsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)DROP TABLE IF EXISTS \"public\"\\.\"llm_billing_settings\"").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db).AllowDestructive()
	if err := down202609171626362410(builder); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
