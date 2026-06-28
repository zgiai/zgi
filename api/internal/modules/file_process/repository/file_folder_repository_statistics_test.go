package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTotalFileCountWithVisibilityAppliesWorkspaceAndFolderAccessFilters(t *testing.T) {
	db, mock, cleanup := newFileFolderRepositoryMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "upload_files" WHERE .*upload_files\.workspace_id.*file_folder_joins.*file_folder_permissions`).
		WithArgs(
			"org-1",
			"workspace-visible",
			string(file_model.FileFolderPermissionAllTeam),
			string(file_model.FileFolderPermissionOnlyMe),
			"account-1",
			string(file_model.FileFolderPermissionPartialTeam),
			"workspace-visible",
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	repo := NewFileFolderRepository(db)
	count, err := repo.GetTotalFileCountWithVisibility(
		t.Context(),
		"org-1",
		"account-1",
		false,
		[]string{"workspace-visible"},
	)
	if err != nil {
		t.Fatalf("GetTotalFileCountWithVisibility error = %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func newFileFolderRepositoryMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatalf("open gorm postgres mock: %v", err)
	}

	return db, mock, func() {
		sqlDB.Close()
	}
}
