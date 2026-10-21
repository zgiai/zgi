package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

// The forward migration that produced fractional points is retired: the storage
// unit is milli-points in BIGINT. The ID must stay registered because the
// current plan validates unknown migrations, so its body is an empty no-op and
// the repair lives in 202610011200000000_restore_milli_point_storage.

func TestRetireInternalCreditUnitsPreMigrationIsNoOp(t *testing.T) {
	db, mock := openMigrationMockDB(t)

	builder := mschema.New(db)
	if err := up202609171652571815(builder); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRetireInternalCreditUnitsPreRollbackIsNoOp(t *testing.T) {
	db, mock := openMigrationMockDB(t)

	builder := mschema.New(db).AllowDestructive()
	if err := down202609171652571815(builder); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRestoreMilliPointStoragePreMigration covers the repair migration: every
// credit column goes back to BIGINT milli-points and the snapshot amount is
// multiplied back with the unit marker cleared.
func TestRestoreMilliPointStoragePreMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)

	for i := 0; i < len(preRetiredCreditColumns()); i++ {
		mock.ExpectExec("(?s)DO \\$\\$.*TYPE bigint.*END \\$\\$").
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("(?s)UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set.*charged_credits.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)UPDATE llm_usage_bills SET pricing_snapshot = \\(pricing_snapshot - 'unit'\\)").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := up202610011200000000(builder); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreMilliPointStoragePreRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)

	for i := 0; i < len(preRetiredCreditColumns()); i++ {
		mock.ExpectExec("(?s)DO \\$\\$.*TYPE numeric\\(20,3\\).*END \\$\\$").
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("(?s)UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set.*charged_credits.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set.*\\{unit\\}.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db).AllowDestructive()
	if err := down202610011200000000(builder); err != nil {
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
	for _, file := range []string{
		"202609171652571815_retire_internal_credit_units.go",
		"202610011200000000_restore_milli_point_storage.go",
	} {
		if err := checkMigrationSourceSafety([]string{file}); err != nil {
			t.Fatalf("migration source safety failed for %s: %v", file, err)
		}
	}
}
