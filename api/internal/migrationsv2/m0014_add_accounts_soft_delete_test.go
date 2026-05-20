package migrationsv2

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestM0014_add_accounts_soft_delete_AddsDeletedAtAndIndex(t *testing.T) {
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

	mock.ExpectExec(`ALTER TABLE accounts\s+ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_accounts_deleted_at\s+ON accounts \(deleted_at\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0014_add_accounts_soft_delete().Migrate(db); err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
