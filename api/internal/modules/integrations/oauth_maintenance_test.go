package integrations

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestOAuthArtifactMaintenanceUsesLeaseAndBoundedSecretCleanup(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres"}), &gorm.Config{
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_try_advisory_xact_lock\(\$1\)`).
		WithArgs(oauthMaintenanceAdvisoryLockID).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_xact_lock"}).AddRow(true))
	mock.ExpectExec(`UPDATE "integration_oauth_flows".*"encrypted_flow_token".*WHERE id IN \(SELECT "id" FROM "integration_oauth_flows".*LIMIT \$[0-9]+\) AND status = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "integration_oauth_flows".*"encrypted_flow_token".*WHERE id IN \(SELECT "id" FROM "integration_oauth_flows".*LIMIT \$[0-9]+\) AND status <> \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "integration_oauth_states" WHERE id IN \(SELECT "id" FROM "integration_oauth_states".*LIMIT \$[0-9]+\)`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM "integration_oauth_flows" WHERE id IN \(SELECT "id" FROM "integration_oauth_flows".*LIMIT \$[0-9]+\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := NewGormOAuthFlowRepository(db).MaintainOAuthArtifacts(
		context.Background(), now, now.Add(-24*time.Hour), 100,
	)
	if err != nil {
		t.Fatalf("MaintainOAuthArtifacts() error = %v", err)
	}
	if !result.LeaseAcquired || result.ExpiredFlows != 1 || result.ClearedFlows != 1 ||
		result.DeletedFlows != 1 || result.DeletedStates != 2 {
		t.Fatalf("maintenance result = %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("maintenance SQL expectations: %v", err)
	}
}

func TestOAuthArtifactMaintenanceSkipsWhenLeaseIsHeldElsewhere(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres"}), &gorm.Config{
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_try_advisory_xact_lock\(\$1\)`).
		WithArgs(oauthMaintenanceAdvisoryLockID).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_xact_lock"}).AddRow(false))
	mock.ExpectCommit()
	result, err := NewGormOAuthFlowRepository(db).MaintainOAuthArtifacts(
		context.Background(), time.Now().UTC(), time.Now().UTC().Add(-24*time.Hour), 100,
	)
	if err != nil {
		t.Fatalf("MaintainOAuthArtifacts() error = %v", err)
	}
	if result.LeaseAcquired || result.ExpiredFlows != 0 || result.ClearedFlows != 0 ||
		result.DeletedFlows != 0 || result.DeletedStates != 0 {
		t.Fatalf("maintenance result without lease = %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("maintenance SQL expectations: %v", err)
	}
}
