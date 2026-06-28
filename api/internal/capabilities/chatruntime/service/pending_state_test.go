package service

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPersistClientActionPendingUpdatesMessageAndConversationInOneTransaction(t *testing.T) {
	db, mock := newPendingStateRepositoryMockDB(t)
	svc := &service{repos: repository.NewRepositories(db)}
	conversationID := uuid.New()
	messageID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message: &runtimemodel.Message{
			ID:       messageID,
			Query:    "打开文件管理",
			Metadata: map[string]interface{}{},
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_messages" SET .* WHERE id = .* AND deleted_at IS NULL AND status IN .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"current_leaf_message_id".* WHERE id = .* AND active_message_id = .* AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	metadata := svc.persistClientActionPending(context.Background(), prepared, map[string]interface{}{
		"action_id":   "route:/console/files",
		"action_type": "route_navigation",
		"skill_id":    "console-navigator",
		"tool_name":   "navigate",
		"href":        "/console/files",
	}, nil)

	continuation := mapFromOperationContext(metadata["client_action_continuation"])
	if got := stringFromAny(continuation["status"]); got != clientActionStatusWaiting {
		t.Fatalf("client_action_continuation status = %q, want %q", got, clientActionStatusWaiting)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPersistToolGovernancePendingUpdatesMessageAndConversationInOneTransaction(t *testing.T) {
	db, mock := newPendingStateRepositoryMockDB(t)
	svc := &service{repos: repository.NewRepositories(db)}
	conversationID := uuid.New()
	messageID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message: &runtimemodel.Message{
			ID:       messageID,
			Query:    "删除这个文件",
			Metadata: map[string]interface{}{},
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_messages" SET .* WHERE id = .* AND deleted_at IS NULL AND status IN .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"current_leaf_message_id".*"dialogue_count"=dialogue_count \+ 1.* WHERE id = .* AND active_message_id = .* AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	metadata := svc.persistToolGovernanceApprovalPending(context.Background(), prepared, map[string]interface{}{
		"correlation_id": "approval-1",
		"skill_id":       "file-manager",
		"tool_name":      "delete_file",
	}, nil)

	continuation := mapFromOperationContext(metadata["tool_governance_continuation"])
	if got := stringFromAny(continuation["status"]); got != "waiting_approval" {
		t.Fatalf("tool_governance_continuation status = %q, want waiting_approval", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPersistPendingStateRollsBackWhenConversationFinishFails(t *testing.T) {
	db, mock := newPendingStateRepositoryMockDB(t)
	svc := &service{repos: repository.NewRepositories(db)}
	conversationID := uuid.New()
	messageID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message: &runtimemodel.Message{
			ID:       messageID,
			Query:    "打开文件管理",
			Metadata: map[string]interface{}{},
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_messages" SET .* WHERE id = .* AND deleted_at IS NULL AND status IN .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .* WHERE id = .* AND active_message_id = .* AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	_ = svc.persistClientActionPending(context.Background(), prepared, map[string]interface{}{
		"action_id":   "route:/console/files",
		"action_type": "route_navigation",
		"skill_id":    "console-navigator",
		"tool_name":   "navigate",
		"href":        "/console/files",
	}, nil)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func newPendingStateRepositoryMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
