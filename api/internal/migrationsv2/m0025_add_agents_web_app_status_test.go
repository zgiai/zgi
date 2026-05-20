package migrationsv2

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestM0025_add_agents_web_app_status_AddsColumnsAndIndex(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}

	mock.ExpectExec(`ALTER TABLE IF EXISTS public\.agents\s+ADD COLUMN IF NOT EXISTS web_app_status VARCHAR\(20\) NOT NULL DEFAULT 'active',\s+ADD COLUMN IF NOT EXISTS web_app_offlined_at TIMESTAMPTZ,\s+ADD COLUMN IF NOT EXISTS web_app_offlined_by UUID,\s+ADD COLUMN IF NOT EXISTS web_app_offline_reason TEXT NOT NULL DEFAULT ''`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_agents_web_app_status\s+ON public\.agents\(web_app_status\)\s+WHERE deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0025_add_agents_web_app_status().Migrate(db); err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
