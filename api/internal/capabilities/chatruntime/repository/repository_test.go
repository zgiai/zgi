package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestFinishWaitingApprovalMessagePromotesLeaf(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	conversationID := uuid.New()
	messageID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"current_leaf_message_id".*"dialogue_count"=dialogue_count \+ 1.* WHERE id = .* AND active_message_id = .* AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), conversationID, messageID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.FinishWaitingApprovalMessage(context.Background(), conversationID, messageID); err != nil {
		t.Fatalf("FinishWaitingApprovalMessage: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFinishContinuationMessageKeepsSameLeafWithoutIncrement(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	conversationID := uuid.New()
	messageID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"current_leaf_message_id".* WHERE id = .* AND active_message_id = .* AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), conversationID, messageID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.FinishContinuationMessage(context.Background(), conversationID, messageID); err != nil {
		t.Fatalf("FinishContinuationMessage: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func newConversationRepositoryMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open gorm mock db: %v", err)
	}
	return db, mock
}
