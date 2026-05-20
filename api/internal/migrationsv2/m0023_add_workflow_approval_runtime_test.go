package migrationsv2

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestM0023_add_workflow_approval_runtime_CreatesApprovalRuntimeTables(t *testing.T) {
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

	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS public\.workflow_approval_forms`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0023_add_workflow_approval_runtime().Migrate(db); err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
