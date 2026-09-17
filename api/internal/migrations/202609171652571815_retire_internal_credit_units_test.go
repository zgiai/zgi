package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestRetireInternalCreditUnitsPreMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)

	for i := 0; i < 10; i++ {
		mock.ExpectExec("(?s)DO \\$\\$.*information_schema.columns.*numeric\\(20,3\\).*END \\$\\$").
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("(?s)UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set.*charged_credits.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set.*\\{unit\\}.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := up202609171652571815(builder); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRetireInternalCreditUnitsPreRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)

	for i := 0; i < 10; i++ {
		mock.ExpectExec("(?s)DO \\$\\$.*numeric_scale = 3.*bigint.*END \\$\\$").
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("(?s)UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set.*charged_credits.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)UPDATE llm_usage_bills SET pricing_snapshot = \\(pricing_snapshot - 'unit'\\)").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db).AllowDestructive()
	if err := down202609171652571815(builder); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRetireInternalCreditUnitsPreStatement(t *testing.T) {
	forward := preAlterColumnsStatement("channel_wallets", []string{"balance"}, true)
	for _, expected := range []string{
		"information_schema.columns",
		"table_name = 'channel_wallets'",
		"column_name IN ('balance')",
		"TYPE numeric(20,3) USING (%I::numeric / 1000)",
		"data_type = 'bigint'",
	} {
		if !strings.Contains(forward, expected) {
			t.Fatalf("forward statement missing %q:\n%s", expected, forward)
		}
	}

	reverse := preAlterColumnsStatement("channel_wallets", []string{"balance"}, false)
	for _, expected := range []string{
		"numeric_scale = 3",
		"TYPE bigint USING ((%I * 1000)::bigint)",
	} {
		if !strings.Contains(reverse, expected) {
			t.Fatalf("reverse statement missing %q:\n%s", expected, reverse)
		}
	}
}

func TestRetireInternalCreditUnitsPreSourceSafety(t *testing.T) {
	if err := checkMigrationSourceSafety([]string{"202609171652571815_retire_internal_credit_units.go"}); err != nil {
		t.Fatalf("migration source safety failed: %v", err)
	}
}
