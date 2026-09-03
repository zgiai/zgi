package integrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGormOAuthClientFlowLockerUsesTransactionScopedPostgreSQLLock(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	organizationID := uuid.New()
	locker := newGormOAuthClientFlowLocker(db)
	lockKey := organizationID.String() + "/fake/user_oauth"

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended\(\$1, 0\)\)`).
		WithArgs(lockKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	operationCalled := false
	err := locker.WithinOAuthClientFlowLock(
		context.Background(),
		organizationID,
		" FAKE ",
		" USER_OAUTH ",
		func(lockedContext context.Context) error {
			transaction, inTransaction := oauthClientFlowDatabase(lockedContext, db)
			if transaction == nil || !inTransaction {
				t.Fatal("OAuth client-flow operation did not receive the locked transaction")
			}
			operationCalled = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("WithinOAuthClientFlowLock() error = %v", err)
	}
	if !operationCalled {
		t.Fatal("OAuth client-flow operation was not called")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
