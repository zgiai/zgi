package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestOfficialModelProviderProvenanceMigration(t *testing.T) {
	const migrationID = "20260715120000_add_official_model_provider_provenance"

	var migrate func(*gorm.DB) error
	for _, candidate := range registeredMigrations() {
		if candidate.ID != migrationID {
			continue
		}
		migrate = candidate.Migrate
		break
	}
	if migrate == nil {
		t.Fatalf("migration %s is not registered", migrationID)
	}

	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_official_model_snapshots.*ADD COLUMN IF NOT EXISTS effective_provider_models jsonb.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
