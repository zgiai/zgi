package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRepositoryGetRunScopedIncludesWorkspaceScope(t *testing.T) {
	db, mock, cleanup := openActionRuntimeRepositoryMockDB(t)
	defer cleanup()
	repo := NewRepository(db)

	mock.ExpectQuery(`SELECT \* FROM "action_runtime_runs" WHERE workspace_id = \$1 AND \(id = \$2 AND organization_id = \$3 AND account_id = \$4 AND deleted_at IS NULL\).*LIMIT \$5`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, _, err := repo.GetRunScoped(context.Background(), uuid.New(), uuid.New(), uuidPtr(uuid.New()), uuid.New())
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetRunScoped workspace error = %v, want not found", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestRepositoryGetRunScopedNilWorkspaceOnlyMatchesNilWorkspace(t *testing.T) {
	db, mock, cleanup := openActionRuntimeRepositoryMockDB(t)
	defer cleanup()
	repo := NewRepository(db)

	mock.ExpectQuery(`SELECT \* FROM "action_runtime_runs" WHERE workspace_id IS NULL AND \(id = \$1 AND organization_id = \$2 AND account_id = \$3 AND deleted_at IS NULL\).*LIMIT \$4`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, _, err := repo.GetRunScoped(context.Background(), uuid.New(), uuid.New(), nil, uuid.New())
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetRunScoped nil workspace error = %v, want not found", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestRepositoryIdempotencyLookupIncludesWorkspaceAndCapability(t *testing.T) {
	db, mock, cleanup := openActionRuntimeRepositoryMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	now := time.Now()
	runID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	accountID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "action_runtime_runs" WHERE workspace_id = \$1 AND \(organization_id = \$2 AND account_id = \$3 AND capability_id = \$4 AND idempotency_key = \$5 AND deleted_at IS NULL\) ORDER BY created_at DESC, id DESC.*LIMIT \$6`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "file.create", "same-key", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"organization_id",
			"workspace_id",
			"account_id",
			"capability_id",
			"idempotency_key",
			"created_at",
			"updated_at",
		}).AddRow(runID, organizationID, workspaceID, accountID, "file.create", "same-key", now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "action_runtime_steps" WHERE run_id = $1 ORDER BY step_index ASC, created_at ASC, id ASC`)).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_id"}))

	run, _, err := repo.GetRunByIdempotencyKey(context.Background(), organizationID, &workspaceID, accountID, "file.create", "same-key")
	if err != nil {
		t.Fatalf("GetRunByIdempotencyKey: %v", err)
	}
	if run.ID != runID {
		t.Fatalf("run id = %s, want %s", run.ID, runID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func openActionRuntimeRepositoryMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
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
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}
	return db, mock, func() { _ = sqlDB.Close() }
}

func uuidPtr(value uuid.UUID) *uuid.UUID {
	return &value
}
