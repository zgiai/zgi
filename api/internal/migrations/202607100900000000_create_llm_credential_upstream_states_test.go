package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateLLMCredentialUpstreamStatesMigrationUpDown(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectQuery(`(?s)SELECT EXISTS .*information_schema\.tables`).
		WithArgs("llm_credential_upstream_states").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	for range 5 {
		mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 1))
	}

	upBuilder := mschema.New(db)
	if err := upCreateLLMCredentialUpstreamStates(upBuilder); err != nil {
		t.Fatalf("run migration up: %v", err)
	}
	statements := strings.Join(upBuilder.Statements(), "\n")
	for _, want := range []string{
		`CREATE TABLE "public"."llm_credential_upstream_states"`,
		`"manual_retry_requested_at" timestamptz`,
		`"provider_error_code" varchar(128)`,
		`"provider_error_status" integer`,
		`REFERENCES "public"."llm_credentials" ("id") ON DELETE CASCADE`,
		"INSERT INTO llm_credential_upstream_states",
	} {
		if !strings.Contains(statements, want) {
			t.Fatalf("up migration missing %q:\n%s", want, statements)
		}
	}

	mock.ExpectExec(`DROP TABLE IF EXISTS "public"\."llm_credential_upstream_states"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	downBuilder := mschema.New(db).AllowDestructive()
	if err := downCreateLLMCredentialUpstreamStates(downBuilder); err != nil {
		t.Fatalf("run migration down: %v", err)
	}
	if got := strings.Join(downBuilder.Statements(), "\n"); !strings.Contains(got, `DROP TABLE IF EXISTS "public"."llm_credential_upstream_states"`) {
		t.Fatalf("down migration missing table drop:\n%s", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
