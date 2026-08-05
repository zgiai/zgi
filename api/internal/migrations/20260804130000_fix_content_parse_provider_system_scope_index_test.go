package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestFixContentParseProviderSystemScopeIndexMigration(t *testing.T) {
	var migrate func(*gorm.DB) error
	for _, candidate := range registeredMigrations() {
		if candidate.ID == migrationFixContentParseProviderSystemScopeIndexID {
			migrate = candidate.Migrate
			break
		}
	}
	if migrate == nil {
		t.Fatalf("migration %s is not registered", migrationFixContentParseProviderSystemScopeIndexID)
	}

	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*DROP INDEX IF EXISTS public\\.uq_content_parse_provider_configs_system_provider.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s).*CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_provider_configs_system_provider.*organization_id IS NULL.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
