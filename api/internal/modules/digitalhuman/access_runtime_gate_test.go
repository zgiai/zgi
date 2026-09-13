package digitalhuman

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestResolveAPIKeyUsesPerResourceAccessWithoutDeploymentGate(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_"+"EXTERNAL_API_ENABLED", "false")
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery(`SELECT .* FROM "digital_human_api_keys".*`).WillReturnError(gorm.ErrRecordNotFound)
	_, _, err = NewAccessService(db).ResolveAPIKey(t.Context(), "zgi_test_key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResolveAPIKey() error = %v, want ErrNotFound after key lookup", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveWebAppUsesPerResourceAccessWithoutDeploymentGate(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_"+"PUBLIC_WEBAPP_ENABLED", "false")
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery(`SELECT .* FROM "digital_humans".*`).WillReturnError(gorm.ErrRecordNotFound)
	_, err = NewAccessService(db).ResolveWebApp(t.Context(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResolveWebApp() error = %v, want ErrNotFound after definition lookup", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
