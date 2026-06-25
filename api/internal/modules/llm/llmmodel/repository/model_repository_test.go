package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openModelRepositoryMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}
	return db, mock
}

func TestListAvailableByNamesRequiresActiveStatus(t *testing.T) {
	db, mock := openModelRepositoryMockDB(t)
	repo := NewModelRepository(db)
	mock.ExpectQuery(`(?s)WHERE .*status`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := repo.ListAvailableByNames(context.Background(), []string{"gpt-4o"}, "", "")
	if err != nil {
		t.Fatalf("ListAvailableByNames returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListAvailableFilteredRequiresActiveStatus(t *testing.T) {
	db, mock := openModelRepositoryMockDB(t)
	repo := NewModelRepository(db)
	mock.ExpectQuery(`(?s)WHERE .*status`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := repo.ListAvailableFiltered(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ListAvailableFiltered returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
