package integrations

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGormExecutionRepositoryCreatesAndCompletesAuditRecord(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres"}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	repository := NewGormExecutionRepository(db)

	now := time.Now().UTC()
	record := &ExecutionRecord{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		AccountID:      uuidPointer(uuid.New()),
		IntegrationID:  IntegrationWebSearch,
		DriverID:       DriverExa,
		ActionID:       ActionWebSearch,
		InvokeFrom:     "aichat",
		Status:         "running",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "integration_executions"`).
		WithArgs(anySQLArgs(22)...).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.Create(context.Background(), record); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	cost := 0.007
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "integration_executions" SET`).
		WithArgs(anySQLArgs(7)...).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.Complete(context.Background(), record.ID, ExecutionCompletion{
		Status:            "succeeded",
		ProviderRequestID: "exa-request",
		DurationMS:        42,
		CostUSD:           &cost,
		ResultCount:       3,
		AttemptCount:      1,
	}); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormExecutionRepositoryCompleteRequiresExistingRecord(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres"}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "integration_executions" SET`).
		WithArgs(anySQLArgs(6)...).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	err = NewGormExecutionRepository(db).Complete(context.Background(), uuid.New(), ExecutionCompletion{Status: "failed"})
	if err == nil {
		t.Fatal("Complete() error = nil, want record-not-found failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func uuidPointer(value uuid.UUID) *uuid.UUID { return &value }

func anySQLArgs(count int) []driver.Value {
	values := make([]driver.Value, count)
	for index := range values {
		values[index] = sqlmock.AnyArg()
	}
	return values
}
