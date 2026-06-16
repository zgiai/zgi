package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
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

func TestSearchByCallerScopedMapsResults(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	updatedAt := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"type",
		"conversation_id",
		"conversation_title",
		"message_id",
		"match_text",
		"updated_at",
		"rank",
	}).AddRow(
		"message",
		conversationID.String(),
		"Release notes",
		messageID.String(),
		"Generate release notes",
		updatedAt,
		1,
	)
	mock.ExpectQuery(`(?s).*chat_runtime_conversations AS c.*chat_runtime_messages AS m.*ORDER BY rank ASC, updated_at DESC.*LIMIT.*`).
		WithArgs(
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			"%release%",
			"%release%",
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			"%release%",
			"%release%",
			20,
		).
		WillReturnRows(rows)

	results, err := repo.SearchByCallerScoped(context.Background(), organizationID, accountID, runtimemodel.ConversationCallerAIChat, nil, "release", 20)
	if err != nil {
		t.Fatalf("SearchByCallerScoped: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ConversationID != conversationID {
		t.Fatalf("conversation id = %s, want %s", results[0].ConversationID, conversationID)
	}
	if results[0].MessageID == nil || *results[0].MessageID != messageID {
		t.Fatalf("message id = %v, want %s", results[0].MessageID, messageID)
	}
	if results[0].MatchText != "Generate release notes" {
		t.Fatalf("match text = %q", results[0].MatchText)
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
