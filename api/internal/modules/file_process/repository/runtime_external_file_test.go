package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestListFilesExcludesRuntimeExternalFiles(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "upload_files" WHERE organization_id = \$1 AND NOT EXISTS \(.*runtime_external_files.*runtime_file\.file_id = upload_files\.id.*\)`).
		WithArgs("org-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT .* FROM "upload_files" WHERE organization_id = \$1 AND NOT EXISTS \(.*runtime_external_files.*runtime_file\.file_id = upload_files\.id.*\).*ORDER BY created_at DESC LIMIT \$2`).
		WithArgs("org-1", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "name", "size", "extension", "created_at"}).
			AddRow("file-1", "org-1", "visible.txt", 1, "txt", time.Now()))
	files, total, err := NewFileRepository(db).ListByTenantIDs(context.Background(), "org-1", "account-1", false, nil, 1, 20, "", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(files) != 1 || files[0].ID != "file-1" {
		t.Fatalf("files/total = %#v/%d", files, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
