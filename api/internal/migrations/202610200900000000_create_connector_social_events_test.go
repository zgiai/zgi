package migrations

import (
	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"strings"
	"testing"
)

func TestConnectorSocialEventsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.connector_social_events.*CREATE INDEX idx_connector_social_events_unread").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up202610200900000000(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("(?s)DROP TABLE IF EXISTS public.connector_social_events").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down202610200900000000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"bigserial PRIMARY KEY", "event_id uuid NOT NULL UNIQUE", "encrypted_payload text NOT NULL", "UNIQUE (connection_id, source_id)", "acknowledged_at IS NULL", "REFERENCES public.connector_connections(id)"} {
		if !strings.Contains(createConnectorSocialEventsSQL, want) {
			t.Fatalf("missing schema guarantee %s", want)
		}
	}
}
