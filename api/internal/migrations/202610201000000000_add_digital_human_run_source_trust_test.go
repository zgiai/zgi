package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddDigitalHumanRunSourceTrustMigrationContract(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ADD COLUMN IF NOT EXISTS source_trust.*ADD CONSTRAINT digital_human_runs_source_trust_check.*'external_end_user'`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upAddDigitalHumanRunSourceTrust(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddDigitalHumanRunSourceTrustPostgres(t *testing.T) {
	db := openExecutionFileTestPostgres(t)
	if err := db.Exec(`CREATE TABLE public.digital_human_runs (id uuid PRIMARY KEY, status text NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := upAddDigitalHumanRunSourceTrust(mschema.New(db)); err != nil {
			t.Fatal(err)
		}
	}
	for _, trust := range []any{nil, "authorized_operator", "workspace_member", "channel_participant", "external_end_user"} {
		if err := db.Exec(`INSERT INTO digital_human_runs (id,status,source_trust) VALUES (?, 'queued', ?)`, uuid.New(), trust).Error; err != nil {
			t.Fatalf("%v: %v", trust, err)
		}
	}
	if err := db.Exec(`INSERT INTO digital_human_runs (id,status,source_trust) VALUES (gen_random_uuid(), 'queued', 'administrator')`).Error; err == nil {
		t.Fatal("unknown source trust was admitted")
	}
}
