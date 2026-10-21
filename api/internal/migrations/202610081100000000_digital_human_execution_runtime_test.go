package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDigitalHumanExecutionRuntimeMigrationContract(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.digital_human_versions.*ADD COLUMN IF NOT EXISTS execution_runtime jsonb.*ck_digital_human_execution_runtime`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upDigitalHumanExecutionRuntime(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDigitalHumanExecutionRuntimePostgres(t *testing.T) {
	db := openExecutionFileTestPostgres(t)
	if err := db.Exec(`CREATE TABLE digital_human_versions (id uuid PRIMARY KEY, revision text NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := db.Exec(`INSERT INTO digital_human_versions VALUES (?, 'legacy-revision')`, id).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := upDigitalHumanExecutionRuntime(mschema.New(db)); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM digital_human_versions WHERE id = ? AND revision = 'legacy-revision' AND execution_runtime IS NULL`, id).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy version changed: count=%d err=%v", count, err)
	}
	for _, invalid := range []string{`{}`, `null`, `{"enabled":null}`, `{"enabled":true,"profile_id":"other"}`, `{"enabled":1}`} {
		if err := db.Exec(`INSERT INTO digital_human_versions VALUES (?, 'new', ?::jsonb)`, uuid.New(), invalid).Error; err == nil {
			t.Fatalf("invalid execution setting persisted: %s", invalid)
		}
	}
	for _, valid := range []string{`{"enabled":true}`, `{"enabled":false}`} {
		if err := db.Exec(`INSERT INTO digital_human_versions VALUES (?, 'new', ?::jsonb)`, uuid.New(), valid).Error; err != nil {
			t.Fatal(err)
		}
	}
}
