package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateIntegrationOperationReceiptsContract(t *testing.T) {
	sql := compactSQL(createIntegrationOperationReceiptsSQL)
	for _, expected := range []string{
		"CREATE TABLE public.integration_operation_receipts",
		"batch_id varchar(128) NOT NULL",
		"operation_item_id varchar(128) NOT NULL",
		"item_index integer NOT NULL",
		"item_count integer NOT NULL",
		"operation_key varchar(64) NOT NULL",
		"target_hmac varchar(64) NOT NULL",
		"frozen_input_hmac varchar(64) NOT NULL",
		"CHECK (status IN ('executing', 'succeeded', 'outcome_unknown'))",
		"CHECK (item_index >= 1 AND item_count >= 1 AND item_index <= item_count)",
		"result_payload jsonb NOT NULL DEFAULT '{}'::jsonb",
		"CREATE UNIQUE INDEX uidx_integration_operation_receipts_org_key ON public.integration_operation_receipts (organization_id, operation_key)",
		"idx_integration_operation_receipts_message",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("operation receipt migration missing %q: %s", expected, sql)
		}
	}
	for _, forbidden := range []string{"message_text", "input_payload", "credentials", "access_token"} {
		if strings.Contains(strings.ToLower(sql), forbidden) {
			t.Fatalf("operation receipt migration must not persist %q", forbidden)
		}
	}
}

func TestCreateIntegrationOperationReceiptsExecutesAndRollbackIsGuarded(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.integration_operation_receipts.*idx_integration_operation_receipts_status_lease").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260801090000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	mock.ExpectExec("(?s)DO.*cannot remove integration operation receipts while replay-protection history exists").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DROP TABLE IF EXISTS "public"\."integration_operation_receipts"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260801090000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}
