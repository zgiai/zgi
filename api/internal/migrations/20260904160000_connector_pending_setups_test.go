package migrations

import (
	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"strings"
	"testing"
)

func TestConnectorPendingForwardMigration(t *testing.T) {
	db, m := openMigrationMockDB(t)
	m.ExpectExec(`(?s)CREATE TABLE public.connector_pending_setups.*ADD COLUMN pending_setup_id.*CREATE UNIQUE INDEX connector_pending_one_attempt`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upConnectorPendingSetups(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	table := strings.Split(connectorPendingSetupsSQL, "ALTER TABLE")[0]
	for _, forbidden := range []string{"app_secret", "encrypted", "device_code", "access_token", "binding_id"} {
		if strings.Contains(table, forbidden) {
			t.Fatal("candidate persistence:", forbidden)
		}
	}
	for _, required := range []string{"protocol_version = 1", "'register_bot','reauthorize'", "'login_session','reauthorize'", "intent = 'update_app'", "intent = 'resume_app'", "pending_completion", "verification_deadline", "source_flow_id uuid NOT NULL UNIQUE"} {
		if !strings.Contains(connectorPendingSetupsSQL, required) {
			t.Fatal("missing migration invariant", required)
		}
	}
	if strings.Contains(connectorPendingSetupsSQL, "DROP TABLE") || strings.Contains(connectorPendingSetupsSQL, "ON DELETE CASCADE") {
		t.Fatal("migration deletes existing data")
	}
}
