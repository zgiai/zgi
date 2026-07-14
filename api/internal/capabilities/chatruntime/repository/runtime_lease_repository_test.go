package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRuntimeLeaseMarksOnlyExpiredRuns(t *testing.T) {
	db, mock := newRuntimeLeaseMockDB(t)
	repo := NewRuntimeLeaseRepository(db)
	expiredID := uuid.New()
	legacyID := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(`(?s)UPDATE "chat_runtime_messages".*runtime_heartbeat_at.*runtime_run_id.*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(expiredID).AddRow(legacyID))

	ids, err := repo.MarkExpiredActiveAsError(
		context.Background(),
		now.Add(-90*time.Second),
		now.Add(-time.Hour),
		"runtime_lease_expired",
	)
	if err != nil {
		t.Fatalf("MarkExpiredActiveAsError: %v", err)
	}
	if len(ids) != 2 || !containsUUID(ids, expiredID) || !containsUUID(ids, legacyID) {
		t.Fatalf("expired ids = %v, want %s and %s", ids, expiredID, legacyID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRuntimeRunOwnershipPreventsOldCompletion(t *testing.T) {
	db, mock := newRuntimeLeaseMockDB(t)
	repo := NewMessageRepository(db)
	messageID := uuid.New()
	staleRunID := uuid.New()
	currentRunID := uuid.New()

	mock.ExpectExec(`(?s)UPDATE "chat_runtime_messages" SET .* WHERE .*id = .*status IN.*runtime_run_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	err := repo.UpdateCompleted(WithRuntimeRunID(context.Background(), staleRunID), messageID, "stale", map[string]interface{}{})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("stale completion error = %v, want record not found", err)
	}

	mock.ExpectExec(`(?s)UPDATE "chat_runtime_messages" SET .* WHERE .*id = .*status IN.*runtime_run_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateCompleted(WithRuntimeRunID(context.Background(), currentRunID), messageID, "current", map[string]interface{}{}); err != nil {
		t.Fatalf("current completion: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRuntimeRunOwnershipPreventsOldError(t *testing.T) {
	db, mock := newRuntimeLeaseMockDB(t)
	repo := NewMessageRepository(db)
	messageID := uuid.New()
	staleRunID := uuid.New()
	currentRunID := uuid.New()

	mock.ExpectExec(`(?s)UPDATE "chat_runtime_messages" SET .* WHERE .*id = .*status IN.*runtime_run_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	err := repo.UpdateError(WithRuntimeRunID(context.Background(), staleRunID), messageID, "stale failure")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("stale error update = %v, want record not found", err)
	}

	mock.ExpectExec(`(?s)UPDATE "chat_runtime_messages" SET .* WHERE .*id = .*status IN.*runtime_run_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateError(WithRuntimeRunID(context.Background(), currentRunID), messageID, "current failure"); err != nil {
		t.Fatalf("current error update: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func newRuntimeLeaseMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open gorm mock db: %v", err)
	}
	return db, mock
}

func containsUUID(values []uuid.UUID, target uuid.UUID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
