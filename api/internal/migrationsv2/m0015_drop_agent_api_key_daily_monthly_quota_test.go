package migrationsv2

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestM0015_drop_agent_api_key_daily_monthly_quota_DropsRetiredColumns(t *testing.T) {
	db, mock, cleanup := openMigrationMockDB(t)
	defer cleanup()

	mock.ExpectExec(`ALTER TABLE IF EXISTS agent_api_keys\s+DROP COLUMN IF EXISTS daily_quota,\s+DROP COLUMN IF EXISTS monthly_quota,\s+DROP COLUMN IF EXISTS daily_usage,\s+DROP COLUMN IF EXISTS monthly_usage,\s+DROP COLUMN IF EXISTS last_reset_date`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0015_drop_agent_api_key_daily_monthly_quota().Migrate(db); err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestM0015_drop_agent_api_key_daily_monthly_quota_IsIdempotent(t *testing.T) {
	db, mock, cleanup := openMigrationMockDB(t)
	defer cleanup()

	mock.ExpectExec(`ALTER TABLE IF EXISTS agent_api_keys\s+DROP COLUMN IF EXISTS daily_quota,\s+DROP COLUMN IF EXISTS monthly_quota,\s+DROP COLUMN IF EXISTS daily_usage,\s+DROP COLUMN IF EXISTS monthly_usage,\s+DROP COLUMN IF EXISTS last_reset_date`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0015_drop_agent_api_key_daily_monthly_quota().Migrate(db); err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func openMigrationMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}

	return db, mock, func() {
		_ = sqlDB.Close()
	}
}
