package repository

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newSetupRepositoryTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
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
		sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}

	cleanup := func() {
		_ = sqlDB.Close()
	}
	return db, mock, cleanup
}

func TestGetSetupStatus_ReturnsNilWhenSetupTableMissing(t *testing.T) {
	db, mock, cleanup := newSetupRepositoryTestDB(t)
	defer cleanup()

	repo := NewSetupRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "zgi_setups" ORDER BY "zgi_setups"."version" LIMIT $1`)).
		WithArgs(1).
		WillReturnError(errors.New(`ERROR: relation "zgi_setups" does not exist (SQLSTATE 42P01)`))

	setup, err := repo.GetSetupStatus()
	if err != nil {
		t.Fatalf("expected nil error when setup table is missing, got %v", err)
	}
	if setup != nil {
		t.Fatalf("expected nil setup when setup table is missing, got %+v", setup)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGetSetupStatus_PropagatesUnexpectedErrors(t *testing.T) {
	db, mock, cleanup := newSetupRepositoryTestDB(t)
	defer cleanup()

	repo := NewSetupRepository(db)
	wantErr := errors.New("db unavailable")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "zgi_setups" ORDER BY "zgi_setups"."version" LIMIT $1`)).
		WithArgs(1).
		WillReturnError(wantErr)

	setup, err := repo.GetSetupStatus()
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
	if setup != nil {
		t.Fatalf("expected nil setup on unexpected error, got %+v", setup)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateSetup_PersistsSetupMarker(t *testing.T) {
	db, mock, cleanup := newSetupRepositoryTestDB(t)
	defer cleanup()

	repo := NewSetupRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "zgi_setups" ("version","setup_at") VALUES ($1,$2)`)).
		WithArgs("1.0", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repo.CreateSetup(); err != nil {
		t.Fatalf("CreateSetup() error = %v, want nil", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
